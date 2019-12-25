package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"path"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"

	"go.skia.org/infra/go/auth"
	docker_pubsub "go.skia.org/infra/go/docker/build/pubsub"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/task_driver/go/lib/auth_steps"
	"go.skia.org/infra/task_driver/go/lib/checkout"
	"go.skia.org/infra/task_driver/go/lib/docker"
	"go.skia.org/infra/task_driver/go/lib/golang"
	"go.skia.org/infra/task_driver/go/lib/os_steps"
	"go.skia.org/infra/task_driver/go/td"
)

var (
	// Required properties for this task.
	gerritProject = flag.String("gerrit_project", "", "Gerrit project name.")
	gerritUrl     = flag.String("gerrit_url", "", "URL of the Gerrit server.")
	projectId     = flag.String("project_id", "", "ID of the Google Cloud project.")
	taskId        = flag.String("task_id", "", "ID of this task.")
	taskName      = flag.String("task_name", "", "Name of the task.")
	workdir       = flag.String("workdir", ".", "Working directory")

	checkoutFlags = checkout.SetupFlags(nil)

	// Optional flags.
	local  = flag.Bool("local", false, "True if running locally (as opposed to on the bots)")
	output = flag.String("o", "", "If provided, dump a JSON blob of step data to the given file. Prints to stdout if '-' is given.")
)

const (
	JSFIDDLE_IMAGE_NAME  = "jsfiddle-v2"
	SKOTTIE_IMAGE_NAME   = "skottie-v2"
	PARTICLES_IMAGE_NAME = "particles-v2"
)

var (
	infraCommonEnv = map[string]string{
		"SKIP_BUILD": "1",
		"ROOT":       "/WORKSPACE",
	}
)

func buildPushJsFiddleImage(ctx context.Context, tag, repo, wasmProductsDir, configDir string, topic *pubsub.Topic) error {
	tempDir, err := os_steps.TempDir(ctx, "", "")
	if err != nil {
		return err
	}
	image := fmt.Sprintf("gcr.io/skia-public/%s", JSFIDDLE_IMAGE_NAME)
	buildCmd := "cd /home/skia/golib/src/go.skia.org/infra/jsfiddle && make release_ci"
	volumes := []string{
		fmt.Sprintf("%s:/OUT", wasmProductsDir),
		fmt.Sprintf("%s:/WORKSPACE", tempDir),
	}
	return docker.BuildPushImageFromInfraV2(ctx, "JsFiddle", buildCmd, image, tag, repo, configDir, tempDir, topic, volumes, infraCommonEnv, nil)
}

func buildPushSkottieImage(ctx context.Context, tag, repo, wasmProductsDir, configDir string, topic *pubsub.Topic) error {
	tempDir, err := os_steps.TempDir(ctx, "", "")
	if err != nil {
		return err
	}
	image := fmt.Sprintf("gcr.io/skia-public/%s", SKOTTIE_IMAGE_NAME)
	buildCmd := "cd /home/skia/golib/src/go.skia.org/infra/skottie && make release_ci"
	volumes := []string{
		fmt.Sprintf("%s:/OUT", wasmProductsDir),
		fmt.Sprintf("%s:/WORKSPACE", tempDir),
	}
	return docker.BuildPushImageFromInfraV2(ctx, "Skottie", buildCmd, image, tag, repo, configDir, tempDir, topic, volumes, infraCommonEnv, nil)
}

func buildPushParticlesImage(ctx context.Context, tag, repo, wasmProductsDir, configDir string, topic *pubsub.Topic) error {
	tempDir, err := os_steps.TempDir(ctx, "", "")
	if err != nil {
		return err
	}
	image := fmt.Sprintf("gcr.io/skia-public/%s", PARTICLES_IMAGE_NAME)
	buildCmd := "cd /home/skia/golib/src/go.skia.org/infra/particles && make release_ci"
	volumes := []string{
		fmt.Sprintf("%s:/OUT", wasmProductsDir),
		fmt.Sprintf("%s:/WORKSPACE", tempDir),
	}
	return docker.BuildPushImageFromInfraV2(ctx, "Particles", buildCmd, image, tag, repo, configDir, tempDir, topic, volumes, infraCommonEnv, nil)
}

func main() {
	// Setup.
	ctx := td.StartRun(projectId, taskId, taskName, output, local)
	defer td.EndRun(ctx)

	rs, err := checkout.GetRepoState(checkoutFlags)
	if err != nil {
		td.Fatal(ctx, err)
	}
	if *gerritProject == "" {
		td.Fatalf(ctx, "--gerrit_project is required.")
	}
	if *gerritUrl == "" {
		td.Fatalf(ctx, "--gerrit_url is required.")
	}

	wd, err := os_steps.Abs(ctx, *workdir)
	if err != nil {
		td.Fatal(ctx, err)
	}

	// Check out the code.
	_, err = checkout.EnsureGitCheckout(ctx, path.Join(wd, "repo"), rs)
	if err != nil {
		td.Fatal(ctx, err)
	}

	// Setup go.
	ctx = golang.WithEnv(ctx, wd)

	// Create token source with scope for cloud registry (storage) and pubsub.
	ts, err := auth_steps.Init(ctx, *local, auth.SCOPE_USERINFO_EMAIL, auth.SCOPE_FULL_CONTROL, pubsub.ScopePubSub)
	if err != nil {
		td.Fatal(ctx, err)
	}

	// Create pubsub client.
	client, err := pubsub.NewClient(ctx, docker_pubsub.TOPIC_PROJECT_ID, option.WithTokenSource(ts))
	if err != nil {
		td.Fatal(ctx, err)
	}
	topic := client.Topic(docker_pubsub.TOPIC)

	// Figure out which tag to use for docker build and push.
	tag := rs.Revision
	if rs.Issue != "" && rs.Patchset != "" {
		tag = fmt.Sprintf("%s_%s", rs.Issue, rs.Patchset)
	}

	// Create a temporary config dir for Docker.
	configDir, err := ioutil.TempDir("", "")
	if err != nil {
		td.Fatal(ctx, err)
	}
	defer util.RemoveAll(configDir)

	// Login to docker (required to push to docker).
	token, err := ts.Token()
	if err != nil {
		td.Fatal(ctx, err)
	}
	if err := docker.Login(ctx, token.AccessToken, "gcr.io/skia-public/", configDir); err != nil {
		td.Fatal(ctx, err)
	}

	// Run skia-wasm-release-v2 image and extract wasm products out of it.
	wasmProductsDir, err := os_steps.TempDir(ctx, "", "")
	if err != nil {
		td.Fatal(ctx, err)
	}
	// Run Doxygen pointing to the location of the checkout and the out dir.
	volumes := []string{
		fmt.Sprintf("%s:/OUT", wasmProductsDir),
	}
	wasmCopyCmd := "cp -r /tmp/* /OUT"
	// TODO(rmistry): Change prod below to tag.
	if err := docker.Run(ctx, "gcr.io/skia-public/skia-wasm-release-v2:prod", wasmCopyCmd, configDir, volumes, nil); err != nil {
		td.Fatal(ctx, err)
	}

	// Build and push all apps of interest below.
	if err := buildPushJsFiddleImage(ctx, tag, rs.Repo, wasmProductsDir, configDir, topic); err != nil {
		td.Fatal(ctx, err)
	}
	if err := buildPushSkottieImage(ctx, tag, rs.Repo, wasmProductsDir, configDir, topic); err != nil {
		td.Fatal(ctx, err)
	}
	if err := buildPushParticlesImage(ctx, tag, rs.Repo, wasmProductsDir, configDir, topic); err != nil {
		td.Fatal(ctx, err)
	}
}
