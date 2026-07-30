package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3 是一个内存版 S3 兼容服务，记录 PutObject / multipart 相关请求，
// 用于断言备份上传走的是单次 PUT 还是分片上传。
type fakeS3 struct {
	mu        sync.Mutex
	putBodies [][]byte          // 单次 PutObject 收到的完整 body
	created   int               // CreateMultipartUpload 调用次数
	parts     map[int][]byte    // UploadPart 收到的分片（partNumber -> body）
	completed int               // CompleteMultipartUpload 调用次数
	aborted   int               // AbortMultipartUpload 调用次数
}

func newFakeS3Server(t *testing.T) (*httptest.Server, *fakeS3) {
	t.Helper()
	fake := &fakeS3{parts: map[int][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		defer fake.mu.Unlock()

		q := r.URL.Query()
		_, hasUploads := q["uploads"]
		uploadID := q.Get("uploadId")
		partNumber := q.Get("partNumber")

		switch {
		case r.Method == http.MethodPost && hasUploads:
			// CreateMultipartUpload
			fake.created++
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
				`<Bucket>backups</Bucket><Key>key</Key><UploadId>fake-upload-id</UploadId>`+
				`</InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && uploadID != "" && partNumber != "":
			// UploadPart
			n, err := strconv.Atoi(partNumber)
			if err != nil {
				http.Error(w, "bad part number", http.StatusBadRequest)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusInternalServerError)
				return
			}
			fake.parts[n] = body
			w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, n))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && uploadID != "":
			// CompleteMultipartUpload
			fake.completed++
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
				`<Location>https://example.com/backups/key</Location>`+
				`<Bucket>backups</Bucket><Key>key</Key><ETag>"final-etag"</ETag>`+
				`</CompleteMultipartUploadResult>`)
		case r.Method == http.MethodDelete && uploadID != "":
			// AbortMultipartUpload
			fake.aborted++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut:
			// PutObject（单次上传）
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusInternalServerError)
				return
			}
			fake.putBodies = append(fake.putBodies, body)
			w.Header().Set("ETag", `"put-etag"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.String(), http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, fake
}

func (f *fakeS3) concatenatedParts() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	var numbers []int
	for n := range f.parts {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	var buf bytes.Buffer
	for _, n := range numbers {
		buf.Write(f.parts[n])
	}
	return buf.Bytes()
}

func newBackupStoreForFakeS3(t *testing.T, endpoint string) *S3BackupStore {
	t.Helper()
	client, err := newS3Client(context.Background(), s3ClientParams{
		Endpoint:        endpoint,
		Region:          "auto",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		ForcePathStyle:  true,
	})
	require.NoError(t, err)
	return &S3BackupStore{client: client, bucket: "backups"}
}

// 大于单个分片大小的备份必须通过 multipart 分片上传：
// S3/R2 单次 PutObject 上限 5GiB，生产备份超过后全部报 EntityTooLarge。
func TestS3BackupStore_Upload_LargeObjectUsesMultipart(t *testing.T) {
	// 20MiB：超过默认分片大小，必须走 multipart
	payload := make([]byte, 20<<20)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	srv, fake := newFakeS3Server(t)
	store := newBackupStoreForFakeS3(t, srv.URL)

	size, err := store.Upload(context.Background(), "backups/2026/07/31/sub2api.sql.gz", bytes.NewReader(payload), "application/gzip")
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), size, "返回大小应为完整流大小")

	assert.Zero(t, len(fake.putBodies), "大对象禁止单次 PutObject（超过 5GiB 会被 S3/R2 拒绝）")
	assert.Equal(t, 1, fake.created, "应发起一次 CreateMultipartUpload")
	assert.GreaterOrEqual(t, len(fake.parts), 2, "20MiB 至少应拆成 2 个分片")
	assert.Equal(t, 1, fake.completed, "应调用 CompleteMultipartUpload 收尾")
	assert.Zero(t, fake.aborted, "成功路径不应 Abort")
	assert.Equal(t, payload, fake.concatenatedParts(), "分片拼接后必须与原始数据一致")
}

// 小于分片大小的备份保持单次 PutObject（兼容所有 S3 实现）。
func TestS3BackupStore_Upload_SmallObjectSinglePut(t *testing.T) {
	payload := bytes.Repeat([]byte("sub2api-backup-"), 1<<16) // 1MiB

	srv, fake := newFakeS3Server(t)
	store := newBackupStoreForFakeS3(t, srv.URL)

	size, err := store.Upload(context.Background(), "backups/small.sql.gz", bytes.NewReader(payload), "application/gzip")
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), size)

	require.Len(t, fake.putBodies, 1, "小对象应单次 PutObject")
	assert.Equal(t, payload, fake.putBodies[0])
	assert.Zero(t, fake.created, "小对象不应走 multipart")
}

// 上传流中途失败时，错误必须透传，且已创建的分片任务应 Abort。
func TestS3BackupStore_Upload_ReaderErrorPropagates(t *testing.T) {
	srv, fake := newFakeS3Server(t)
	store := newBackupStoreForFakeS3(t, srv.URL)

	// 前 17MiB 正常（触发 multipart），之后报错
	payload := make([]byte, 17<<20)
	reader := io.MultiReader(bytes.NewReader(payload), &failReader{err: io.ErrUnexpectedEOF})

	_, err := store.Upload(context.Background(), "backups/broken.sql.gz", reader, "application/gzip")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "UnexpectedEOF") || fake.aborted > 0,
		"读取失败应透传错误并 Abort 未完成的分片任务, err=%v aborted=%d", err, fake.aborted)
}

type failReader struct{ err error }

func (f *failReader) Read([]byte) (int, error) { return 0, f.err }
