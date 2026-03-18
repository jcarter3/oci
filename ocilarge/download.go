package ocilarge

import (
	"context"
	"fmt"
	"hash"
	"io"

	"github.com/jcarter3/oci"
	"github.com/opencontainers/go-digest"
)

func DownloadLargeBlob(ctx context.Context, reg oci.Interface, repo string, digest oci.Digest) (oci.BlobReader, error) {
	desc, err := reg.ResolveBlob(ctx, repo, digest)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve blob: %w", err)
	}
	var chunkSize int64 = 1024 * 1024 * 100 // 100MB
	buf := make([]byte, chunkSize)
	pr, pw := io.Pipe()
	go func() {
		var start int64
		for ; start < desc.Size; start += chunkSize {
			end := start + chunkSize
			if end > desc.Size {
				end = desc.Size
			}
			for i := 0; i < 3; i++ { // retry up to 3 times
				br, err := reg.GetBlobRange(ctx, repo, digest, start, end)
				if err != nil {
					continue
				}
				_, err = br.Read(buf)
				if err != nil {
					_ = br.Close()
					continue
				}
			}
			if err != nil { // if err is set, the retries failed
				pw.CloseWithError(err)
				return
			}
			_, _ = pw.Write(buf)

		}
		pw.Close()
	}()

	return &blobReader{
		r:        pr,
		digester: desc.Digest.Algorithm().Hash(),
		desc:     desc,
		verify:   true,
	}, nil
}

type blobReader struct {
	r        io.ReadCloser
	n        int64
	digester hash.Hash
	desc     oci.Descriptor
	verify   bool
}

func (r *blobReader) Descriptor() oci.Descriptor {
	return r.desc
}

func (r *blobReader) Read(buf []byte) (int, error) {
	n, err := r.r.Read(buf)
	r.n += int64(n)
	r.digester.Write(buf[:n])
	if err == nil {
		if r.n > r.desc.Size {
			// Fail early when the blob is too big; we can do that even
			// when we're not verifying for other use cases.
			return n, fmt.Errorf("blob size exceeds content length %d: %w", r.desc.Size, oci.ErrSizeInvalid)
		}
		return n, nil
	}
	if err != io.EOF {
		return n, err
	}
	if !r.verify {
		return n, io.EOF
	}
	if r.n != r.desc.Size {
		return n, fmt.Errorf("blob size mismatch (%d/%d): %w", r.n, r.desc.Size, oci.ErrSizeInvalid)
	}
	gotDigest := digest.NewDigest(r.desc.Digest.Algorithm(), r.digester)
	if gotDigest != r.desc.Digest {
		return n, fmt.Errorf("digest mismatch when reading blob")
	}
	return n, io.EOF
}

func (r *blobReader) Close() error {
	return r.r.Close()
}
