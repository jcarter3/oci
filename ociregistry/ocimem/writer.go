// Copyright 2023 CUE Labs AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocimem

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/jcarter3/oci/ociregistry"
	"github.com/jcarter3/oci/ociregistry/ociref"
	"github.com/opencontainers/go-digest"
)

// This file implements the ociregistry.Writer methods.

// PushBlob pushes a blob to the named repository.
func (r *Registry) PushBlob(ctx context.Context, repoName string, desc ociregistry.Descriptor, content io.Reader) (ociregistry.Descriptor, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("cannot read content: %v", err)
	}
	if err := CheckDescriptor(desc, data); err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
	return desc, nil
}

// PushBlobChunked starts a chunked blob upload to the named repository.
func (r *Registry) PushBlobChunked(ctx context.Context, repoName string, chunkSize int) (ociregistry.BlobWriter, error) {
	// TODO(mvdan): Why does the ocimem implementation allow a PATCH on an upload ID which doesn't exist?
	// The tests in ociserver make this assumption, so they break without this bit of code.
	//
	// Ideally they should start a new chunked upload to get a new ID, then use that for PATCH/PUT.
	// Alternatively, add a new method to ocimem outside of the interface to start a chunked upload with a predefined ID.
	// Either way, this case should be an error, per the spec.
	return r.PushBlobChunkedResume(ctx, repoName, "", 0, chunkSize)
}

// PushBlobChunkedResume resumes a previously started chunked blob upload.
func (r *Registry) PushBlobChunkedResume(ctx context.Context, repoName, id string, offset int64, chunkSize int) (ociregistry.BlobWriter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return nil, err
	}
	b := repo.uploads[id]
	if b == nil {
		b = NewBuffer(func(b *Buffer) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			desc, data, _ := b.GetBlob()
			repo.blobs[desc.Digest] = &blob{mediaType: desc.MediaType, data: data}
			return nil
		}, id)
		repo.uploads[b.ID()] = b
	}
	b.checkStartOffset = offset
	return b, nil
}

// MountBlob makes a blob from one repository available in another.
func (r *Registry) MountBlob(ctx context.Context, fromRepo, toRepo string, dig ociregistry.Digest) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rto, err := r.makeRepo(toRepo)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	b, err := r.blobForDigest(fromRepo, dig)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	rto.blobs[dig] = b
	return b.descriptor(), nil
}

var errCannotOverwriteTag = fmt.Errorf("%w: cannot overwrite tag", ociregistry.ErrDenied)

// PushManifest pushes a manifest to the named repository, optionally tagging it.
func (r *Registry) PushManifest(ctx context.Context, repoName string, data []byte, mediaType string, params *ociregistry.PushManifestParameters) (ociregistry.Descriptor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	repo, err := r.makeRepo(repoName)
	if err != nil {
		return ociregistry.Descriptor{}, err
	}
	var dig ociregistry.Digest
	if params != nil && params.Digest != "" {
		// Validate the provided digest against the contents.
		if err := params.Digest.Validate(); err != nil {
			return ociregistry.Descriptor{}, fmt.Errorf("invalid digest: %v: %w", err, ociregistry.ErrDigestInvalid)
		}
		verifier := params.Digest.Verifier()
		if _, err := verifier.Write(data); err != nil {
			return ociregistry.Descriptor{}, fmt.Errorf("cannot verify digest: %v", err)
		}
		if !verifier.Verified() {
			return ociregistry.Descriptor{}, fmt.Errorf("digest mismatch: %w", ociregistry.ErrDigestInvalid)
		}
		dig = params.Digest
	} else {
		dig = digest.FromBytes(data)
	}
	desc := ociregistry.Descriptor{
		Digest:    dig,
		MediaType: mediaType,
		Size:      int64(len(data)),
	}
	var tags []string
	if params != nil {
		tags = params.Tags
	}
	for _, tag := range tags {
		if !ociref.IsValidTag(tag) {
			return ociregistry.Descriptor{}, fmt.Errorf("invalid tag")
		}
		if r.cfg.ImmutableTags {
			if currDesc, ok := repo.tags[tag]; ok {
				if dig == currDesc.Digest {
					if currDesc.MediaType != mediaType {
						// Same digest but mismatched media type.
						return ociregistry.Descriptor{}, fmt.Errorf("%w: mismatched media type", ociregistry.ErrDenied)
					}
					// It's got the same content already. Allow it.
					return currDesc, nil
				}
				return ociregistry.Descriptor{}, errCannotOverwriteTag
			}
		}
	}
	// make a copy of the data to avoid potential corruption.
	data = slices.Clone(data)
	if params == nil || params.Digest == "" {
		// Only check descriptor with data when using canonical digest,
		// since CheckDescriptor uses canonical hashing.
		if err := CheckDescriptor(desc, data); err != nil {
			return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: %v", err)
		}
	} else {
		// For non-canonical digests, check descriptor without data verification
		// (we already validated the digest above).
		if desc.MediaType == "" {
			return ociregistry.Descriptor{}, fmt.Errorf("invalid descriptor: no media type in descriptor")
		}
	}
	info, err := r.checkManifestReferences(repoName, desc.MediaType, data)
	if err != nil {
		return ociregistry.Descriptor{}, fmt.Errorf("invalid manifest: %v", err)
	}

	repo.manifests[dig] = &blob{
		mediaType: mediaType,
		data:      data,
		info:      info,
	}
	for _, tag := range tags {
		repo.tags[tag] = desc
	}
	return desc, nil
}

func (r *Registry) checkManifestReferences(repoName string, mediaType string, data []byte) (manifestInfo, error) {
	info, err := getManifestInfo(mediaType, data)
	if err != nil {
		// TODO decide what to do about errUnknownManifestMediaTypeForIteration
		return manifestInfo{}, err
	}
	if r.cfg.LaxChildReferences {
		return info, nil
	}
	repo, err := r.repo(repoName)
	if err != nil {
		return manifestInfo{}, err
	}
	for info := range info.descriptors {
		if err := CheckDescriptor(info.desc, nil); err != nil {
			return manifestInfo{}, fmt.Errorf("bad descriptor in %s: %v", info.name, err)
		}
		switch info.kind {
		case kindBlob:
			if repo.blobs[info.desc.Digest] == nil {
				return manifestInfo{}, fmt.Errorf("blob for %s not found", info.name)
			}
		case kindManifest:
			if repo.manifests[info.desc.Digest] == nil {
				return manifestInfo{}, fmt.Errorf("manifest for %s not found", info.name)
			}
		case kindSubjectManifest:
			// The standard explicitly specifies that we can have
			// a dangling subject so don't check that it exists.
		}
	}
	return info, nil
}

// refersTo reports whether the given digest is referred to, directly or indirectly, by any item
// returned by the given iterator, within the given repository.
// TODO currently this iterates through all tagged manifests. A better
// algorithm could amortise that work and be considerably more efficient.
func refersTo(repo *repository, iter descIter, digest ociregistry.Digest) (found bool, retErr error) {
	for info := range iter {
		if info.desc.Digest == digest {
			return true, nil
		}
		switch info.kind {
		case kindManifest, kindSubjectManifest:
			b := repo.manifests[info.desc.Digest]
			if b == nil {
				break
			}
			minfo, err := getManifestInfo(info.desc.MediaType, b.data)
			if err != nil {
				return false, err
			}
			found, err := refersTo(repo, minfo.descriptors, digest)
			if found || err != nil {
				return found, err
			}
		}
	}
	return false, nil
}
