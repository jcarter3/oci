package ocibuilder

import (
	"context"
	"os"
	"testing"

	"github.com/jcarter3/oci/ociregistry/ociauth"
	"github.com/jcarter3/oci/ociregistry/ociclient"
)

func Test_Boop(t *testing.T) {
	repo := "jcarter3/index_test"

	cf, err := ociauth.Load(nil)
	if err != nil {
		panic(err)
	}

	ocl, err := ociclient.New("index.docker.io", &ociclient.Options{
		DebugID: repo,
		Transport: ociauth.NewStdTransport(ociauth.StdTransportParams{
			Config: cf,
		}),
	})

	bigfile, _ := os.Open("bigfile.txt")

	builder := New(ocl, repo)
	builder.SetArtifactType("application/super-big-llm")
	err = builder.PushLayer("llm_thing", bigfile, nil)
	desc, err := builder.Push(context.Background(), "latest")
}
