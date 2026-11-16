package filter

import (
	"bytes"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	mesheryctlflags "github.com/meshery/meshery/mesheryctl/internal/cli/pkg/flags"
	"github.com/meshery/meshery/mesheryctl/pkg/utils"
)

func TestViewCmd(t *testing.T) {
	mesheryctlflags.InitValidators(FilterCmd)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Not able to get current working directory")
	}
	currDir := filepath.Dir(filename)

	ListTests := []utils.MesheryListCommandTest{
		{
			Name:             "Fetch Filter View",
			Args:             []string{"view", "KumaTest"},
			ExpectedResponse: "view.filter.output.golden",
			Fixture:          "view.filter.api.response.golden",
			URL:              "/api/filter",
			ExpectError:      false,
		},
		{
			Name:             "Fetch Kuma Filter View with ID",
			Args:             []string{"view", "957fbc9b-a655-4892-823d-375102a9587c"},
			ExpectedResponse: "view.id.filter.output.golden",
			Fixture:          "view.id.filter.api.response.golden",
			URL:              "/api/filter/957fbc9b-a655-4892-823d-375102a9587c",
			ExpectError:      false,
		},
	}

	loggerTests := []utils.MesheryCommandTest{
		{
			Name:             "Fetch Filter View for non existing filter",
			Args:             []string{"view", "xyz"},
			ExpectedResponse: "view.nonexisting.filter.output.golden",
			Fixture:          "view.nonexisting.filter.api.response.golden",
			URL:              "/api/filter",
			HttpMethod:       "GET",
			HttpStatusCode:   200,
			ExpectError:      false,
		},
	}

	utils.InvokeMesheryctlTestListCommand(t, update, FilterCmd, ListTests, currDir, "filter")
	utils.InvokeMesheryctlTestCommand(t, update, FilterCmd, loggerTests, currDir, "filter")
}

func TestFilterViewSaveCreatesFileWithExtension(t *testing.T) {
	mesheryctlflags.InitValidators(FilterCmd)

	const filterID = "957fbc9b-a655-4892-823d-375102a9587c"

	mesheryDir := utils.IsolateMesheryHome(t)

	run := utils.MesheryCommandRun{
		Cmd:        FilterCmd,
		Args:       []string{"view", filterID, "--output-format", "json", "--save"},
		CommandDir: utils.CallerDir(t),
		Mocks: []utils.MockURL{
			{
				URL:      "/api/filter",
				Response: "view.filter.api.response.golden",
			},
			{
				URL:      "/api/filter/" + filterID,
				Response: "view.id.filter.api.response.golden",
			},
		},
	}
	if _, err := utils.RunMesheryctlCommand(t, run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	utils.AssertFileExists(t, filepath.Join(mesheryDir, "filter_KumaTest_957fbc9b.json"))
}

func TestFilterViewNameResolution(t *testing.T) {
	mesheryctlflags.InitValidators(FilterCmd)

	t.Run("substring match is not accepted and reports not found", func(t *testing.T) {
		testContext := utils.InitTestEnvironment(t)
		defer utils.StopMockery(t)
		logBuf := utils.SetupMeshkitLoggerTesting(t, false)
		utils.TokenFlag = utils.GetToken(t)

		// API returns oauth2-proxy when search=auth
		apiResp := `{"page":0,"pageSize":10,"totalCount":1,"filters":[{"id":"11111111-1111-4111-8111-111111111111","name":"oauth2-proxy","owner":"owner1"}]}`
		httpmock.RegisterResponder(http.MethodGet, testContext.BaseURL+"/api/filter",
			httpmock.NewStringResponder(http.StatusOK, apiResp))

		var outBuf bytes.Buffer
		FilterCmd.SetOut(&outBuf)
		FilterCmd.SetArgs([]string{"view", "auth", "--output-format", "json"})
		defer utils.ResetCommandFlags(FilterCmd, t)

		err := FilterCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(logBuf.String(), `filter with name: "auth" not found`) {
			t.Fatalf("expected not found log message, got: %s", logBuf.String())
		}
		if strings.Contains(outBuf.String(), "oauth2-proxy") {
			t.Fatalf("did not expect oauth2-proxy in output, got: %s", outBuf.String())
		}
	})

	t.Run("exact match is prioritized over preceding substring match", func(t *testing.T) {
		testContext := utils.InitTestEnvironment(t)
		defer utils.StopMockery(t)
		_ = utils.SetupMeshkitLoggerTesting(t, false)
		utils.TokenFlag = utils.GetToken(t)

		// API returns oauth2-proxy first, but auth second
		apiResp := `{"page":0,"pageSize":10,"totalCount":2,"filters":[{"id":"11111111-1111-4111-8111-111111111111","name":"oauth2-proxy","owner":"owner1"},{"id":"22222222-2222-4222-8222-222222222222","name":"auth","owner":"owner2"}]}`
		httpmock.RegisterResponder(http.MethodGet, testContext.BaseURL+"/api/filter",
			httpmock.NewStringResponder(http.StatusOK, apiResp))

		var outBuf bytes.Buffer
		FilterCmd.SetOut(&outBuf)
		FilterCmd.SetArgs([]string{"view", "auth", "--output-format", "json"})
		defer utils.ResetCommandFlags(FilterCmd, t)

		err := FilterCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(outBuf.String(), "22222222-2222-4222-8222-222222222222") {
			t.Fatalf("expected exact match filter to be displayed, got: %s", outBuf.String())
		}
		if strings.Contains(outBuf.String(), "11111111-1111-4111-8111-111111111111") {
			t.Fatalf("did not expect substring match filter to be displayed, got: %s", outBuf.String())
		}
	})

	t.Run("case-insensitive whole-name match succeeds", func(t *testing.T) {
		testContext := utils.InitTestEnvironment(t)
		defer utils.StopMockery(t)
		_ = utils.SetupMeshkitLoggerTesting(t, false)
		utils.TokenFlag = utils.GetToken(t)

		apiResp := `{"page":0,"pageSize":10,"totalCount":1,"filters":[{"id":"33333333-3333-4333-8333-333333333333","name":"KumaTest","owner":"owner1"}]}`
		httpmock.RegisterResponder(http.MethodGet, testContext.BaseURL+"/api/filter",
			httpmock.NewStringResponder(http.StatusOK, apiResp))

		var outBuf bytes.Buffer
		FilterCmd.SetOut(&outBuf)
		FilterCmd.SetArgs([]string{"view", "kumatest", "--output-format", "json"})
		defer utils.ResetCommandFlags(FilterCmd, t)

		err := FilterCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(outBuf.String(), "33333333-3333-4333-8333-333333333333") {
			t.Fatalf("expected case-insensitive match filter to be displayed, got: %s", outBuf.String())
		}
	})

	t.Run("multiple matching filters prompt user and display selected filter", func(t *testing.T) {
		testContext := utils.InitTestEnvironment(t)
		defer utils.StopMockery(t)
		_ = utils.SetupMeshkitLoggerTesting(t, false)
		utils.TokenFlag = utils.GetToken(t)

		origRunSelectPrompt := runSelectPrompt
		defer func() {
			runSelectPrompt = origRunSelectPrompt
		}()

		promptCalled := false
		runSelectPrompt = func(label string, items []string) (int, error) {
			promptCalled = true
			if len(items) != 2 {
				t.Fatalf("expected 2 prompt items, got %d", len(items))
			}
			return 1, nil // choose second filter
		}

		apiResp := `{"page":0,"pageSize":10,"totalCount":2,"filters":[{"id":"11111111-1111-4111-8111-111111111111","name":"auth","owner":"owner1"},{"id":"22222222-2222-4222-8222-222222222222","name":"auth","owner":"owner2"}]}`
		httpmock.RegisterResponder(http.MethodGet, testContext.BaseURL+"/api/filter",
			httpmock.NewStringResponder(http.StatusOK, apiResp))

		var outBuf bytes.Buffer
		FilterCmd.SetOut(&outBuf)
		FilterCmd.SetArgs([]string{"view", "auth", "--output-format", "json"})
		defer utils.ResetCommandFlags(FilterCmd, t)

		err := FilterCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !promptCalled {
			t.Fatalf("expected select prompt to be called")
		}
		if !strings.Contains(outBuf.String(), "22222222-2222-4222-8222-222222222222") {
			t.Fatalf("expected chosen filter (index 1) to be displayed, got: %s", outBuf.String())
		}
	})

	t.Run("multiple matching filters with prompt cancellation returns error", func(t *testing.T) {
		testContext := utils.InitTestEnvironment(t)
		defer utils.StopMockery(t)
		_ = utils.SetupMeshkitLoggerTesting(t, false)
		utils.TokenFlag = utils.GetToken(t)

		origRunSelectPrompt := runSelectPrompt
		defer func() {
			runSelectPrompt = origRunSelectPrompt
		}()

		runSelectPrompt = func(label string, items []string) (int, error) {
			return 0, utils.ErrPromptCancelled()
		}

		apiResp := `{"page":0,"pageSize":10,"totalCount":2,"filters":[{"id":"11111111-1111-4111-8111-111111111111","name":"auth","owner":"owner1"},{"id":"22222222-2222-4222-8222-222222222222","name":"auth","owner":"owner2"}]}`
		httpmock.RegisterResponder(http.MethodGet, testContext.BaseURL+"/api/filter",
			httpmock.NewStringResponder(http.StatusOK, apiResp))

		FilterCmd.SetArgs([]string{"view", "auth", "--output-format", "json"})
		defer utils.ResetCommandFlags(FilterCmd, t)

		err := FilterCmd.Execute()
		if err == nil {
			t.Fatalf("expected error on prompt cancellation, got nil")
		}
	})
}
