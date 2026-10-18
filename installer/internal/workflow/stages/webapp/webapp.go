package webapp

import (
	"context"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// The prompt panel wraps the title and sizes the options below it, so the
	// note goes on its own lines under the question.
	promptTitle = "Install the WebApp?\n\nWebApp is available from axem as part of a subscription."
	answerYes   = "Yes"
	answerNo    = "No"

	lookupTimeout = 15 * time.Second
)

// Stage asks whether the webapp is installed. It runs before Harbor is
// populated, so a declined webapp's image is not uploaded.
func Stage() core.Stage {
	return core.Stage{
		Name: "select webapp",
		Steps: []core.Step{
			{
				Name: "select webapp",
				Run:  selectWebApp,
			},
		},
	}
}

// selectWebApp asks whether the webapp is installed. It is pre-selected only
// when the cluster already runs it, so accepting the default on an update keeps
// the earlier answer.
func selectWebApp(rt *core.Runtime) error {
	current := answerNo
	if installed(context.Background(), rt.Cluster.Client, rt.Detailf) {
		current = answerYes
	}

	selected, err := rt.Reporter.Select(promptTitle, current, []string{answerYes, answerNo})
	if err != nil {
		return err
	}

	rt.WebApp.Enabled = selected == answerYes
	rt.Detailf("install webapp: %t", rt.WebApp.Enabled)

	return nil
}

// installed reports whether the cluster runs the webapp. A cluster that cannot
// be read is treated as not running it.
func installed(ctx context.Context, client kubernetes.Interface, logf func(format string, args ...any)) bool {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	_, err := client.AppsV1().Deployments(shaide.Namespace).Get(ctx, shaide.WebAppName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logf("could not read the %s Deployment (%v); not pre-selecting the webapp", shaide.WebAppName, err)
		}
		return false
	}

	return true
}
