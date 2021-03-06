package alerts

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.skia.org/infra/go/paramtools"
	"go.skia.org/infra/perf/go/types"
)

const (
	// BadAlertID is the value of an Alert.ID if it is invalid, i.e. hasn't
	// been stored yet.
	//
	// TODO(jcgregorio) Make Alert.ID its own type and BadAlertID and
	// instance of that type.
	BadAlertID = -1
)

var (
	// DefaultSparse is the default value for Config.Sparse.
	DefaultSparse = false
)

// Alert represents the configuration for one alert.
type Alert struct {
	ID             int64                             `json:"id"               datastore:",noindex"`
	DisplayName    string                            `json:"display_name"     datastore:",noindex"`
	Query          string                            `json:"query"            datastore:",noindex"` // The query to perform on the trace store to select the traces to alert on.
	Alert          string                            `json:"alert"            datastore:",noindex"` // Email address or id of a chat room to send alerts to.
	Interesting    float32                           `json:"interesting"      datastore:",noindex"` // The regression interestingness threshold.
	BugURITemplate string                            `json:"bug_uri_template" datastore:",noindex"` // URI Template used for reporting bugs. Format TBD.
	Algo           types.RegressionDetectionGrouping `json:"algo"             datastore:",noindex"` // Which clustering algorithm to use.
	Step           types.StepDetection               `json:"step"             datastore:",noindex"` // Which algorithm to use to detect steps.
	State          ConfigState                       `json:"state"`                                 // The state of the config.
	Owner          string                            `json:"owner"            datastore:",noindex"` // Email address of the person that owns this alert.
	StepUpOnly     bool                              `json:"step_up_only"     datastore:",noindex"` // If true then only steps up will trigger an alert. [Deprecated, use Direction.]
	Direction      Direction                         `json:"direction"        datastore:",noindex"` // Which direction will trigger an alert.
	Radius         int                               `json:"radius"           datastore:",noindex"` // How many commits to each side of a commit to consider when looking for a step. 0 means use the server default.
	K              int                               `json:"k"                datastore:",noindex"` // The K in k-means clustering. 0 means use an algorithmically chosen value based on the data.
	GroupBy        string                            `json:"group_by"         datastore:",noindex"` // A comma separated list of keys in the paramset that all Clustering should be broken up across. Keys must not appear in Query.
	Sparse         bool                              `json:"sparse"           datastore:",noindex"` // Data is sparse, so only include commits that have data.
	MinimumNum     int                               `json:"minimum_num"      datastore:",noindex"` // How many traces need to be found interesting before an alert is fired.
	Category       string                            `json:"category"         datastore:",noindex"` // Which category this alert falls into.
}

// IDToString returns the alerts ID formatted as a string.
func (c *Alert) IDToString() string {
	return IDToString(c.ID)
}

// SetIDFromString sets the Alerts ID to the parsed value of the string.
//
// An invalid alert id (-1) will be set if the string can't be parsed.
func (c *Alert) SetIDFromString(s string) {
	c.ID = StringToID(s)
}

// IDToString returns the alerts ID formatted as a string.
func IDToString(alertID int64) string {
	return strconv.FormatInt(alertID, 10)
}

// StringToID returns the int64 alert ID of the parsed value of the string.
//
// An invalid alert id (-1) will be returned if the string can't be parsed.
func StringToID(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	return i
}

// GroupedBy returns the parsed GroupBy value as a slice of strings.
func (c *Alert) GroupedBy() []string {
	ret := []string{}
	for _, s := range strings.Split(c.GroupBy, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ret = append(ret, s)
	}
	return ret
}

// KeyValue holds a single Params key and value, used in 'Combination'.
type KeyValue struct {
	Key   string
	Value string
}

// Combination is a slice of KeyValue's, returned from GroupCombinations.
type Combination []KeyValue

// equal returns true if the two slices of ints are equal.
func equal(sliceA, sliceB []int) bool {
	for i, a := range sliceA {
		if a != sliceB[i] {
			return false
		}
	}
	return true
}

// inc() will cycle through all combinations of slices with integer values <=
// the values in limits.
//
// I.e. inc increments the values in 'a' up the to maximum values in 'limits',
// effectively counting as if each column in the slice was a different base.
//
// I.e. inc([0,0,1], [1,1,1]) would return [0,1,0].
//
// See the unit tests for more examples.
func inc(a, limits []int) []int {
	ret := make([]int, len(a))
	_ = copy(ret, a)
	for i := len(a) - 1; i >= 0; i-- {
		ret[i] = ret[i] + 1
		if ret[i] <= limits[i] {
			break
		}
		ret[i] = 0
	}
	return ret
}

// toCombination converts the slice of offsets into a Combination.
func toCombination(offsets []int, keys []string, ps paramtools.ParamSet) (Combination, error) {
	ret := Combination{}
	for i, offset := range offsets {
		key := keys[i]
		values, ok := ps[key]
		if !ok {
			return nil, fmt.Errorf("Key %q not found in ParamSet %#v", key, ps)
		}
		ret = append(ret, KeyValue{
			Key:   key,
			Value: values[offset],
		})
	}
	return ret, nil
}

// GroupCombinations returns a slice of Combinations that represent
// all the GroupBy combinations possible for the given ParamSet.
//
// I.e. for:
//	ps := paramtools.ParamSet{
//		"model":  []string{"nexus4", "nexus6", "nexus6"},
//		"config": []string{"565", "8888", "nvpr"},
//		"arch":   []string{"ARM", "x86"},
//	}
//
// the GroupCombinations for a GroupBy of "config, arch" would be:
//
//	[]Combination{
//		Combination{KeyValue{"arch", "ARM"}, KeyValue{"config", "565"}},
//		Combination{KeyValue{"arch", "ARM"}, KeyValue{"config", "8888"}},
//		Combination{KeyValue{"arch", "ARM"}, KeyValue{"config", "nvpr"}},
//		Combination{KeyValue{"arch", "x86"}, KeyValue{"config", "565"}},
//		Combination{KeyValue{"arch", "x86"}, KeyValue{"config", "8888"}},
//		Combination{KeyValue{"arch", "x86"}, KeyValue{"config", "nvpr"}},
//	}
//
func (c *Alert) GroupCombinations(ps paramtools.ParamSet) ([]Combination, error) {
	limits := []int{}
	keys := c.GroupedBy()
	for _, key := range keys {
		limits = append(limits, len(ps[key])-1)
	}
	ret := []Combination{}
	zeroes := make([]int, len(limits))
	cfg := make([]int, len(limits))
	for {
		comb, err := toCombination(cfg, keys, ps)
		if err != nil {
			return nil, fmt.Errorf("Failed to build combination: %s", err)
		}
		ret = append(ret, comb)
		cfg = inc(cfg, limits)
		if equal(cfg, zeroes) {
			break
		}
	}
	return ret, nil
}

// QueriesFromParamset uses GroupCombinations to produce the full set of
// queries that this Config represents.
func (c *Alert) QueriesFromParamset(paramset paramtools.ParamSet) ([]string, error) {
	ret := []string{}
	if len(c.GroupBy) != 0 {
		allCombinations, err := c.GroupCombinations(paramset)
		if err != nil {
			return nil, fmt.Errorf("Failed to build GroupBy combinations: %s", err)
		}
		for _, combo := range allCombinations {
			parsed, err := url.ParseQuery(c.Query)
			if err != nil {
				return nil, fmt.Errorf("Found invalid query %q: %s", c.Query, err)
			}
			for _, kv := range combo {
				parsed[kv.Key] = []string{kv.Value}
			}
			ret = append(ret, parsed.Encode())
		}
	} else {
		ret = append(ret, c.Query)
	}
	return ret, nil
}

// Validate returns true of the Alert is valid.
func (c *Alert) Validate() error {
	parsed, err := url.ParseQuery(c.Query)
	if err != nil {
		return fmt.Errorf("Invalid Config: Invalid Query: %s", err)
	}
	if c.GroupBy != "" {
		for _, groupParam := range c.GroupedBy() {
			if _, ok := parsed[groupParam]; ok {
				return fmt.Errorf("Invalid Config: GroupBy must not appear in Query: %q %q ", c.GroupBy, c.Query)
			}
		}
	}
	if c.StepUpOnly {
		c.StepUpOnly = false
		c.Direction = UP
	}
	return nil
}

// NewConfig creates a new Config properly initialized.
func NewConfig() *Alert {
	return &Alert{
		ID:     BadAlertID,
		Algo:   types.KMeansGrouping,
		State:  ACTIVE,
		Sparse: DefaultSparse,
	}
}
