package ocibuilder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jcarter3/oci/ociregistry"
	"github.com/opencontainers/go-digest"
)

type ImageBuilder struct {
	repository string
	client     ociregistry.Interface
	manifest   ManifestOrIndex
}

func New(c ociregistry.Interface, repository string) *ImageBuilder {
	return &ImageBuilder{
		repository: repository,
		client:     c,
		manifest: ManifestOrIndex{
			SchemaVersion: 2,
			MediaType:     "application/vnd.oci.image.manifest.v1+json",
		},
	}
}

func (ib *ImageBuilder) SetArtifactType(artifactType string) {
	ib.manifest.ArtifactType = artifactType
}

func (ib *ImageBuilder) SetSubject(subject *ociregistry.Descriptor) {
	ib.manifest.Subject = subject
}

func (ib *ImageBuilder) SetConfig(config ociregistry.Descriptor) error {
	if len(ib.manifest.Manifests) > 0 {
		return errors.New("cannot set config on index manifest")
	}
	ib.manifest.Config = &config
	return nil
}

func (ib *ImageBuilder) AddLayer(layer ociregistry.Descriptor) error {
	if len(ib.manifest.Manifests) > 0 {
		return errors.New("cannot add layers to an index manifest")
	}
	ib.manifest.Layers = append(ib.manifest.Layers, layer)
	return nil
}

func (ib *ImageBuilder) PushLayer(mediaType string, reader io.ReadCloser, annotations map[string]string) error {
	defer reader.Close()
	CHUNK_SIZE := 100 * 1024 * 1024 // 100 MB?
	bw, _ := ib.client.PushBlobChunked(context.Background(), ib.repository, CHUNK_SIZE)
	buf := make([]byte, CHUNK_SIZE)
	dgstr := digest.Canonical.Digester()
	for {
		n, err := io.ReadFull(reader, buf)
		if err == io.EOF {
			break
		}
		dgstr.Hash().Write(buf[:n])

		for i := 0; i < 3; i++ { // try writing each chunk three times
			_, err = bw.Write(buf[:n])
			if err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("writing chunk: %w", err)
		}
	}
	dgst := dgstr.Digest()
	desc, err := bw.Commit(dgst)
	if err != nil {
		return fmt.Errorf("committing chunk: %w", err)
	}
	desc.MediaType = mediaType
	for k, v := range annotations {
		desc.Annotations[k] = v
	}
	return ib.AddLayer(desc)
}

func (ib *ImageBuilder) AddManifest(manifest ociregistry.Descriptor) error {
	if len(ib.manifest.Layers) > 0 {
		return errors.New("cannot add manifest to an manifest")
	}
	ib.manifest.MediaType = "application/vnd.oci.image.index.v1+json"
	ib.manifest.Manifests = append(ib.manifest.Manifests, manifest)
	return nil
}

func (ib *ImageBuilder) AddAnnotation(key, value string) {
	ib.manifest.Annotations[key] = value
}

func (ib *ImageBuilder) Push(ctx context.Context, tag string) (ociregistry.Descriptor, error) {
	b, err := json.Marshal(ib.manifest)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("marshaling manifest: %w", err)
	}
	return ib.client.PushManifest(ctx, ib.repository, tag, b, ib.manifest.MediaType)
}
