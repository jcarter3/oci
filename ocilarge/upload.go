package ocilarge

import (
	"context"
	"fmt"
	"io"

	"github.com/jcarter3/oci"
	"github.com/opencontainers/go-digest"
)

func UploadLargeBlob(reg oci.Interface, repo string, f io.ReadCloser, chunkSize int) (oci.Descriptor, error) {
	defer f.Close()
	if chunkSize <= 0 {
		chunkSize = 100 * 1024 * 1024 // 100 MB
	}
	bw, _ := reg.PushBlobChunked(context.Background(), repo, chunkSize)
	buf := make([]byte, chunkSize)
	dgstr := digest.Canonical.Digester()
	for {
		n, err := io.ReadFull(f, buf)
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
			return oci.Descriptor{}, fmt.Errorf("writing chunk: %w", err)
		}
	}
	dgst := dgstr.Digest()
	return bw.Commit(dgst)
}
