package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	harborerrors "github.com/axem-solutions/ai_platform/installer/internal/harbor/errors"
	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

func ListRepositories(ctx context.Context, client *Client, projectName string) ([]Repository, error) {
	q := url.Values{
		"page":      {"1"},
		"page_size": {"100"},
	}

	path := fmt.Sprintf(
		"/api/v2.0/projects/%s/repositories",
		url.PathEscape(projectName),
	)

	repos, err := httpapi.Request[[]Repository](ctx, client.Http, "GET", path, q, nil)
	if err != nil {
		return nil, &harborerrors.Error{
			Kind:    harborerrors.ClassifyError(err),
			Op:      "list repositories",
			Project: projectName,
			Err:     err,
		}
	}

	return repos, nil
}

func ListProjects(ctx context.Context, client *Client) ([]Project, error) {
	q := url.Values{
		"page":      {"1"},
		"page_size": {"100"},
	}

	projects, err := httpapi.Request[[]Project](ctx, client.Http, "GET", "/api/v2.0/projects", q, nil)
	if err != nil {
		return nil, &harborerrors.Error{
			Kind:    harborerrors.ClassifyError(err),
			Op:      "list projects",
			Project: "",
			Err:     err,
		}
	}

	return projects, nil
}

func ListArtifacts(
	ctx context.Context,
	client *Client,
	projectName, repositoryName string,
) ([]Artifact, error) {
	q := url.Values{
		"page":      {"1"},
		"page_size": {"100"},
	}

	path := fmt.Sprintf(
		"/api/v2.0/projects/%s/repositories/%s/artifacts",
		url.PathEscape(projectName),
		url.PathEscape(repositoryName),
	)

	artifacts, err := httpapi.Request[[]Artifact](ctx, client.Http, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, &harborerrors.Error{
			Kind:       harborerrors.ClassifyError(err),
			Op:         "list artifacts",
			Project:    projectName,
			Repository: repositoryName,
			Err:        err,
		}
	}

	return artifacts, nil
}

func ListRobotAccounts(
	ctx context.Context,
	client *Client,
) ([]RobotAccount, error) {
	q := url.Values{
		"page":      {"1"},
		"page_size": {"100"},
	}

	robots, err := httpapi.Request[[]RobotAccount](ctx, client.Http, http.MethodGet, "/api/v2.0/robots", q, nil)
	if err != nil {
		return nil, &harborerrors.Error{
			Kind: harborerrors.ClassifyError(err),
			Op:   "list robot accounts",
			Err:  err,
		}
	}

	return robots, nil
}

func DeleteRepository(
	ctx context.Context,
	client *Client,
	projectName, repositoryName string,
) error {
	path := fmt.Sprintf(
		"/api/v2.0/projects/%s/repositories/%s",
		url.PathEscape(projectName),
		url.PathEscape(repositoryName),
	)

	_, err := httpapi.Request[struct{}](ctx, client.Http, "DELETE", path, nil, nil)
	if err != nil {
		return &harborerrors.Error{
			Kind:       harborerrors.ClassifyError(err),
			Op:         "delete repository",
			Project:    projectName,
			Repository: repositoryName,
			Err:        err,
		}
	}

	return nil
}

func CreateProject(
	ctx context.Context,
	client *Client,
	req CreateProjectRequest,
) error {
	_, err := httpapi.Request[struct{}](ctx, client.Http, http.MethodPost, "/api/v2.0/projects", nil, req)
	if err != nil {
		return &harborerrors.Error{
			Kind:    harborerrors.ClassifyError(err),
			Op:      "create project",
			Project: req.Name,
			Err:     err,
		}
	}

	return nil
}

func UpdateProjectVisibility(
	ctx context.Context,
	client *Client,
	projectName string,
	public bool,
) error {
	path := fmt.Sprintf(
		"/api/v2.0/projects/%s",
		url.PathEscape(projectName),
	)

	_, err := httpapi.Request[struct{}](ctx, client.Http, http.MethodPut, path, nil,
		UpdateProjectRequest{Metadata: ProjectMetadata{Public: strconv.FormatBool(public)}})
	if err != nil {
		return &harborerrors.Error{
			Kind:    harborerrors.ClassifyError(err),
			Op:      "update project visibility",
			Project: projectName,
			Err:     err,
		}
	}

	return nil
}

func CreateRobotAccount(
	ctx context.Context,
	client *Client,
	req CreateRobotAccountRequest,
) (RobotAccount, error) {
	robot, err := httpapi.Request[RobotAccount](ctx, client.Http, http.MethodPost, "/api/v2.0/robots", nil, req)
	if err != nil {
		return RobotAccount{}, &harborerrors.Error{
			Kind: harborerrors.ClassifyError(err),
			Op:   "create robot account",
			Err:  err,
		}
	}

	return robot, nil
}

func SetRobotPassword(
	ctx context.Context,
	client *Client,
	robotID int64,
	req SetRobotPasswordRequest,
) error {
	path := fmt.Sprintf(
		"/api/v2.0/robots/%s",
		strconv.FormatInt(robotID, 10),
	)

	_, err := httpapi.Request[struct{}](ctx, client.Http, http.MethodPatch, path, nil, req)
	if err != nil {
		return &harborerrors.Error{
			Kind: harborerrors.ClassifyError(err),
			Op:   "set robot password",
			Err:  err,
		}
	}

	return nil
}
