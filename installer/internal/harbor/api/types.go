package api

import "time"

type Project struct {
	ID           int64     `json:"project_id"`
	Name         string    `json:"name"`
	OwnerID      int64     `json:"owner_id"`
	OwnerName    string    `json:"owner_name"`
	RepoCount    int64     `json:"repo_count"`
	CreationTime time.Time `json:"creation_time"`
	UpdateTime   time.Time `json:"update_time"`
}

type Repository struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	ProjectID     int64     `json:"project_id"`
	PullCount     int64     `json:"pull_count"`
	ArtifactCount int64     `json:"artifact_count"`
	CreationTime  time.Time `json:"creation_time"`
	UpdateTime    time.Time `json:"update_time"`
}

type Artifact struct {
	ID                int64                   `json:"id"`
	Type              string                  `json:"type"`
	ArtifactType      string                  `json:"artifact_type"`
	MediaType         string                  `json:"media_type"`
	ManifestMediaType string                  `json:"manifest_media_type"`
	Digest            string                  `json:"digest"`
	Icon              string                  `json:"icon"`
	Size              int64                   `json:"size"`
	ProjectID         int64                   `json:"project_id"`
	RepositoryID      int64                   `json:"repository_id"`
	RepositoryName    string                  `json:"repository_name"`
	PushTime          time.Time               `json:"push_time"`
	PullTime          time.Time               `json:"pull_time"`
	Tags              []Tag                   `json:"tags"`
	Labels            []Label                 `json:"labels"`
	Accessories       []ArtifactAccessory     `json:"accessories"`
	AdditionLinks     map[string]ArtifactLink `json:"addition_links"`
	ExtraAttrs        ArtifactExtraAttrs      `json:"extra_attrs"`
	Annotations       map[string]string       `json:"annotations"`
	References        []ArtifactReference     `json:"references"`
}

type Tag struct {
	ID           int64     `json:"id"`
	ArtifactID   int64     `json:"artifact_id"`
	RepositoryID int64     `json:"repository_id"`
	Name         string    `json:"name"`
	PushTime     time.Time `json:"push_time"`
	PullTime     time.Time `json:"pull_time"`
	Immutable    bool      `json:"immutable"`
	Signed       bool      `json:"signed"`
	CreationTime time.Time `json:"creation_time"`
}

type Label struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ArtifactReference struct {
	ID          int64  `json:"id"`
	ChildDigest string `json:"child_digest"`
	Platform    struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"platform"`
}

type ArtifactAccessory struct{}

type ArtifactLink struct {
	Absolute bool   `json:"absolute"`
	Href     string `json:"href"`
}

type ArtifactExtraAttrs struct {
	Architecture string              `json:"architecture"`
	Author       string              `json:"author"`
	Created      time.Time           `json:"created"`
	OS           string              `json:"os"`
	Config       ArtifactExtraConfig `json:"config"`
}

type ArtifactExtraConfig struct {
	ArgsEscaped bool     `json:"ArgsEscaped"`
	Cmd         []string `json:"Cmd"`
	Env         []string `json:"Env"`
}

type CreateProjectRequest struct {
	Name   string `json:"project_name"`
	Public bool   `json:"public"`
}

type UpdateProjectRequest struct {
	Metadata ProjectMetadata `json:"metadata"`
}

type ProjectMetadata struct {
	Public string `json:"public"`
}

type RobotAccount struct {
	ID           int64             `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Level        string            `json:"level"`
	Duration     int64             `json:"duration"`
	Disabled     bool              `json:"disabled"`
	Permissions  []RobotPermission `json:"permissions"`
	CreationTime time.Time         `json:"creation_time"`
	UpdateTime   time.Time         `json:"update_time"`
	ExpiresAt    int64             `json:"expires_at"`
}

type CreateRobotAccountRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Level       string            `json:"level"`
	Duration    int64             `json:"duration"`
	Disabled    bool              `json:"disabled"`
	Permissions []RobotPermission `json:"permissions"`
}

type RobotPermission struct {
	Kind      string        `json:"kind"`
	Namespace string        `json:"namespace"`
	Access    []RobotAccess `json:"access"`
}

type RobotAccess struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
}

type SetRobotPasswordRequest struct {
	Secret string `json:"secret"`
}
