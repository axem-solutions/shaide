package knowledgecenter

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
	promptTitle = "Install the Knowledge Center?\n\nKnowledge Center is available from axem as part of a subscription."
	answerYes   = "Yes"
	answerNo    = "No"

	lookupTimeout = 15 * time.Second
)

// Stage asks whether the Knowledge Center is installed. It runs before Harbor
// is populated, so a declined Knowledge Center's images are not uploaded.
func Stage() core.Stage {
	return core.Stage{
		Name: "select Knowledge Center",
		Steps: []core.Step{
			{
				Name: "select Knowledge Center",
				Run:  selectKnowledgeCenter,
			},
		},
	}
}

// selectKnowledgeCenter asks whether the Knowledge Center is installed. One
// answer covers KC and its MCP server. It is pre-selected only when the
// cluster already runs KC, so accepting the default on an update keeps the
// earlier answer.
func selectKnowledgeCenter(rt *core.Runtime) error {
	current := answerNo
	if installed(context.Background(), rt.Cluster.Client, rt.Detailf) {
		current = answerYes
	}

	selected, err := rt.Reporter.Select(promptTitle, current, []string{answerYes, answerNo})
	if err != nil {
		return err
	}

	rt.KnowledgeCenter.Enabled = selected == answerYes
	rt.Detailf("install Knowledge Center: %t", rt.KnowledgeCenter.Enabled)

	return nil
}

// installed reports whether the cluster runs the Knowledge Center. A cluster
// that cannot be read is treated as not running it.
func installed(ctx context.Context, client kubernetes.Interface, logf func(format string, args ...any)) bool {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	_, err := client.AppsV1().Deployments(shaide.Namespace).Get(ctx, shaide.KnowledgeCenterName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logf("could not read the %s Deployment (%v); not pre-selecting the Knowledge Center", shaide.KnowledgeCenterName, err)
		}
		return false
	}

	return true
}
