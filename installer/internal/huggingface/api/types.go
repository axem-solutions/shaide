package api

import (
	"fmt"
	"strings"
)

type Request struct {
	RepoID   string
	Revision string
}

type Metadata struct {
	ID         string
	SHA        string
	Revision   string
	TotalBytes int64
	TotalFiles int
	Siblings   []Sibling
}

type Sibling struct {
	Filename string
	Size     int64
}

type Response struct {
	ID       string            `json:"id"`
	SHA      string            `json:"sha"`
	Siblings []ResponseSibling `json:"siblings"`
}

type ResponseSibling struct {
	Filename string `json:"rfilename"`
	Size     int64  `json:"size"`
}

type SizeInfo struct {
	TotalBytes int64
	TotalFiles int
}

func newRequest(repoID, revision, defaultRevision string) (Request, error) {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return Request{}, fmt.Errorf("repo id is required")
	}

	revision = strings.TrimSpace(revision)
	if revision == "" {
		revision = strings.TrimSpace(defaultRevision)
	}
	if revision == "" {
		return Request{}, fmt.Errorf("revision is required")
	}

	return Request{
		RepoID:   repoID,
		Revision: revision,
	}, nil
}

func FromResponse(revision string, response Response) Metadata {
	metadata := Metadata{
		ID:       response.ID,
		SHA:      response.SHA,
		Revision: revision,
		Siblings: make([]Sibling, 0, len(response.Siblings)),
	}

	for _, sibling := range response.Siblings {
		metadata.Siblings = append(metadata.Siblings, Sibling{
			Filename: sibling.Filename,
			Size:     sibling.Size,
		})

		if sibling.Size > 0 {
			metadata.TotalBytes += sibling.Size
		}
	}

	metadata.TotalFiles = len(metadata.Siblings)

	return metadata
}

func sizeFromMetadata(metadata Metadata) SizeInfo {
	return SizeInfo{
		TotalBytes: metadata.TotalBytes,
		TotalFiles: metadata.TotalFiles,
	}
}
