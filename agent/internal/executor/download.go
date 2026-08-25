package executor

import (
	"context"
	"net/http"

	"github.com/manchtools/cadestro/sdk/sys/remote"
)

var remoteHTTPClient *http.Client

var fetchArtifact = func(ctx context.Context, url, dest, checksum, mode string, redirect remote.RedirectPolicy) error {

	src, err := remote.NewHTTP(remote.HTTPConfig{
		URL:            url,
		ChecksumSHA256: checksum,
		Mode:           mode,
		Redirect:       redirect,
		Client:         remoteHTTPClient,
	})
	if err != nil {
		return err
	}
	_, err = src.Fetch(ctx, dest)
	return err
}

func redirectForArtifact(checksum string) remote.RedirectPolicy {
	if checksum != "" {
		return remote.RedirectCrossOrigin
	}
	return remote.RedirectSameOrigin
}
