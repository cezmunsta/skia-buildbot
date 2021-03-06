package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	autoroll_status "go.skia.org/infra/autoroll/go/status"
	"go.skia.org/infra/go/common"
	"go.skia.org/infra/go/ds"
	"go.skia.org/infra/go/httputils"
	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/sklog"
)

// Autoroller - describes an autoroller to query autoroll_status with.
// Eg: ID: "skia-autoroll", Host: "autoroll.skia.org".
type Autoroller struct {
	ID   string
	Host string
}

// AutorollerSnapshot - contains the current state of an autoroller with
// it's display name (eg: "Chrome") and URL (eg: "https://autoroll.skia.org/r/skia-autoroll").
type AutorollerSnapshot struct {
	DisplayName string `json:"name"`
	NumFailed   int    `json:"num_failed"`
	Url         string `json:"url"`
}

var (
	// nameToAutoroller is a map of autoroller display names to their ID and hosts.
	nameToAutoroller = map[string]*Autoroller{
		"ANGLE":       {ID: "angle-skia-autoroll", Host: "autoroll.skia.org"},
		"Android":     {ID: "android-master-autoroll", Host: "skia-autoroll.corp.goog"},
		"Chrome":      {ID: "skia-autoroll", Host: "autoroll.skia.org"},
		"Google3":     {ID: "google3-autoroll", Host: "skia-autoroll.corp.goog"},
		"Flutter":     {ID: "skia-flutter-autoroll", Host: "autoroll.skia.org"},
		"Skcms":       {ID: "skcms-skia-autoroll", Host: "autoroll.skia.org"},
		"SwiftShader": {ID: "swiftshader-skia-autoroll", Host: "autoroll.skia.org"},
	}

	// Channel that will determine which rollers need to be watched.
	// Entries will look like this: "Android, Chrome, Flutter".
	rollersToWatch = make(chan string, 1)
)

func getAutorollersSnapshot(ctx context.Context) ([]*AutorollerSnapshot, error) {
	autorollersSnapshot := []*AutorollerSnapshot{}
	for name, autoroller := range nameToAutoroller {
		s, err := autoroll_status.Get(ctx, autoroller.ID)
		if err != nil {
			return nil, fmt.Errorf("Could not get the status of %s: %s", autoroller.ID, err)
		}
		snapshot := &AutorollerSnapshot{
			DisplayName: name,
			NumFailed:   s.AutoRollMiniStatus.NumFailedRolls,
			Url:         fmt.Sprintf("https://%s/r/%s", autoroller.Host, autoroller.ID),
		}
		autorollersSnapshot = append(autorollersSnapshot, snapshot)
	}
	sort.Slice(autorollersSnapshot, func(i, j int) bool {
		return autorollersSnapshot[i].DisplayName < autorollersSnapshot[j].DisplayName
	})
	return autorollersSnapshot, nil
}

func StartWatchingAutorollers(rollers string) {
	rollersToWatch <- rollers
}

func StopWatchingAutorollers() {
	// Empty the RollersToWatch channel.
L:
	for {
		select {
		case <-rollersToWatch:
		default:
			break L
		}
	}
}

func AutorollersInit(ctx context.Context, ts oauth2.TokenSource) error {
	if err := ds.InitWithOpt(common.PROJECT_ID, ds.AUTOROLL_NS, option.WithTokenSource(ts)); err != nil {
		return skerr.Wrapf(err, "Failed to initialize Cloud Datastore for autorollers")
	}

	// Start goroutine to watch for rollers.
	go func() {
		for {
			rollers := <-rollersToWatch
			sklog.Infof("Checking for rollers: %s", rollers)
			if rollers == "" {
				continue
			}
			rollsLanded := true
			rollerNames := strings.Split(rollers, ", ")
			for _, rollerName := range rollerNames {
				roller := nameToAutoroller[rollerName]
				s, err := autoroll_status.Get(ctx, roller.ID)
				if err != nil {
					sklog.Errorf("Could not get status of %s: %s\n", roller, err)
					// Continue so that we can try again.
					rollsLanded = false
					continue
				}
				if s.AutoRollMiniStatus.NumFailedRolls == 0 {
					sklog.Infof("Roller %s has 0 NumFailedRolls\n", roller)
					rollsLanded = rollsLanded && true
					continue
				} else {
					sklog.Infof("Roller %s has %d NumFailedRolls. Continue the loop.\n", roller, s.AutoRollMiniStatus.NumFailedRolls)
					rollsLanded = false
					continue
				}
			}
			if rollsLanded {
				// Send status notification.
				rollerText := "roller"
				if len(rollerNames) > 1 {
					rollerText = fmt.Sprintf("%ss", rollerText)
				}
				message := fmt.Sprintf("Open: %s %s landed", rollers, rollerText)
				sklog.Infof("Sending status notification with message: \"%s\"", message)
				if err := AddStatus(message, "tree-status@skia.org", OPEN_STATE, ""); err != nil {
					sklog.Infof("Failed to add automated message to the datastore: %s", err)
				}
			} else {
				rollersToWatch <- rollers
				sklog.Info("Sleeping for 10 seconds")
				time.Sleep(10 * time.Second)
			}
		}
	}()

	return nil
}

// HTTP Handlers.

func (srv *Server) autorollersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	as, err := getAutorollersSnapshot(r.Context())
	if err != nil {
		httputils.ReportError(w, err, "Failed to get autoroll statuses.", http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(as); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}
