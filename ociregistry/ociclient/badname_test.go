package ociclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadRepoName(t *testing.T) {
	ctx := context.Background()
	r, err := New("never.used", &Options{
		Insecure:  true,
		Transport: noTransport{},
	})
	require.NoError(t, err)
	_, err = r.GetBlob(ctx, "Invalid--Repo", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	assert.Regexp(t, "invalid OCI request: name invalid: invalid repository name", err.Error())
	_, err = r.GetBlob(ctx, "okrepo", "bad-digest")
	assert.Regexp(t, "invalid OCI request: digest invalid: badly formed digest", err.Error())
	_, err = r.ResolveTag(ctx, "okrepo", "bad-Tag!")
	assert.Regexp(t, "invalid OCI request: 404 Not Found: page not found", err.Error())
}

type noTransport struct{}

func (noTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no can do")
}
