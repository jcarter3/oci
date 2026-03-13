package ocibuilder

import "github.com/jcarter3/oci/ociregistry"

// ManifestOrIndex parses the required fields out of a manifest json file. It handles indexes and manifests
type ManifestOrIndex struct {
	SchemaVersion int `json:"schemaVersion"`

	// MediaType specifies the type of this document data structure e.g. `application/vnd.oci.image.manifest.v1+json` // TODO: add validation... if index, make sure it has manifests instead of layers?
	MediaType string `json:"mediaType,omitempty"`

	// ArtifactType specifies the IANA media type of artifact when the manifest is used for an artifact.
	ArtifactType string `json:"artifactType,omitempty"`

	// Manifests references platform specific manifests.
	Manifests []ociregistry.Descriptor `json:"manifests"`

	// Config references a configuration object for a container, by digest.
	// The referenced configuration object is a JSON blob that the runtime uses to set up the container.
	Config *ociregistry.Descriptor `json:"config"`

	// Layers is an indexed list of layers referenced by the manifest.
	Layers []ociregistry.Descriptor `json:"layers"`

	// Subject is an optional link from the image manifest to another manifest forming an association between the image manifest and the other manifest.
	Subject *ociregistry.Descriptor `json:"subject,omitempty"`

	// Annotations contains arbitrary metadata for the image manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
}
