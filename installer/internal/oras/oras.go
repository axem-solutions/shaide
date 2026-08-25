package oras

import "github.com/axem-solutions/ai_platform/installer/internal/oras/uploader"

type UploaderOptions = uploader.UploaderOptions

type Uploader = uploader.Uploader

func NewUploader(opts UploaderOptions) (*Uploader, error) {
	return uploader.NewUploader(opts)
}
