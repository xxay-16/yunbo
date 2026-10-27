package openlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrCannotResume = errors.New("无法续传")
var ErrMultipartUnavailable = errors.New("服务端未启用上传续传接口")

type DownloadState struct {
	Initialized  bool   `json:"initialized"`
	Resumable    bool   `json:"resumable"`
	Total        int64  `json:"total"`
	Modified     string `json:"modified"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// DownloadResumable validates object metadata and HTTP byte ranges before
// appending anything. A server returning 200 to a resume request is never appended.
func (c *Client) DownloadResumable(ctx context.Context, remote, password string, dst *os.File, state *DownloadState, checkpoint func(DownloadState) error, progress Progress) error {
	link, file, err := c.DownloadLink(ctx, remote, password)
	if err != nil {
		return err
	}
	stat, err := dst.Stat()
	if err != nil {
		return err
	}
	offset := stat.Size()
	if state.Initialized {
		if file.Size != state.Total || file.Modified != state.Modified {
			return fmt.Errorf("%w：远端文件已变化，请重新下载", ErrCannotResume)
		}
		if offset > state.Total {
			return fmt.Errorf("%w：本地临时文件长度异常", ErrCannotResume)
		}
		if offset == state.Total {
			if progress != nil {
				progress(offset, state.Total)
			}
			return nil
		}
		if offset > 0 && !state.Resumable {
			return fmt.Errorf("%w：源服务器不支持 Range 下载", ErrCannotResume)
		}
	} else if offset != 0 {
		return fmt.Errorf("%w：临时文件缺少来源校验信息", ErrCannotResume)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		if state.ETag != "" {
			req.Header.Set("If-Range", state.ETag)
		} else if state.LastModified != "" {
			req.Header.Set("If-Range", state.LastModified)
		}
	}
	resp, err := c.transfer.Do(req)
	if err != nil {
		return requestError(err)
	}
	defer resp.Body.Close()
	total := file.Size
	if resp.StatusCode == http.StatusPartialContent {
		start, end, size, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil || start != offset || size != file.Size || end != size-1 || (resp.ContentLength >= 0 && resp.ContentLength != end-start+1) {
			return fmt.Errorf("%w：服务器返回的分段范围与文件不一致", ErrCannotResume)
		}
		total = size
	} else if resp.StatusCode != http.StatusOK {
		return &HTTPError{Status: resp.StatusCode}
	} else if offset > 0 {
		return fmt.Errorf("%w：服务器忽略了 Range 或文件已经变化，已阻止拼接", ErrCannotResume)
	} else if resp.ContentLength >= 0 {
		total = resp.ContentLength
		if total != file.Size {
			return fmt.Errorf("%w：文件元数据与下载长度不一致", ErrCannotResume)
		}
	}
	etag := resp.Header.Get("ETag")
	if strings.HasPrefix(etag, "W/") {
		etag = ""
	}
	lastModified := resp.Header.Get("Last-Modified")
	if state.Initialized && offset > 0 {
		if (state.ETag != "" && etag != state.ETag) || (state.LastModified != "" && lastModified != state.LastModified) {
			return fmt.Errorf("%w：远端文件校验标识已变化", ErrCannotResume)
		}
	}
	*state = DownloadState{Initialized: true, Resumable: resp.StatusCode == http.StatusPartialContent, Total: total, Modified: file.Modified, ETag: etag, LastModified: lastModified}
	if err = checkpoint(*state); err != nil {
		return err
	}
	if _, err = dst.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	if progress != nil {
		progress(offset, total)
	}
	reader := &progressReader{reader: resp.Body, total: total, done: offset, progress: progress}
	n, err := io.Copy(dst, reader)
	if err != nil {
		return err
	}
	if offset+n != total {
		return fmt.Errorf("下载不完整：应接收 %d 字节，实际 %d 字节", total, offset+n)
	}
	return nil
}

func parseContentRange(value string) (start, end, total int64, err error) {
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, errors.New("invalid Content-Range")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 {
		return 0, 0, 0, errors.New("invalid Content-Range")
	}
	bounds := strings.Split(parts[0], "-")
	if len(bounds) != 2 {
		return 0, 0, 0, errors.New("invalid Content-Range")
	}
	start, err = strconv.ParseInt(bounds[0], 10, 64)
	if err != nil {
		return
	}
	end, err = strconv.ParseInt(bounds[1], 10, 64)
	if err != nil {
		return
	}
	total, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	if start < 0 || end < start || total <= end {
		err = errors.New("invalid Content-Range")
	}
	return
}

type MultipartSnapshot struct {
	ID            string   `json:"upload_id"`
	State         string   `json:"state"`
	Path          string   `json:"path"`
	Size          int64    `json:"size"`
	ChunkSize     int64    `json:"chunk_size"`
	TotalChunks   int      `json:"total_chunks"`
	Received      [][2]int `json:"received"`
	ReceivedBytes int64    `json:"received_bytes"`
	Frontier      int      `json:"frontier"`
	Error         string   `json:"error,omitempty"`
	Resumed       bool     `json:"resumed"`
}

func (c *Client) multipartCall(ctx context.Context, method, route string, query url.Values, headers map[string]string, body io.Reader, length int64, out any) error {
	u := *c.base
	u.Path += route
	u.RawPath = ""
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	if body != nil {
		req.ContentLength = length
	}
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.api.Do(req)
	if err != nil {
		return requestError(err)
	}
	return decodeResponse(resp, out)
}

// A missing session expressed by a JSON 404 confirms the route exists;
// HTTP 404 and SPA HTML do not.
func (c *Client) ProbeUploadResume(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var snapshot MultipartSnapshot
	err := c.multipartCall(ctx, "GET", "/api/fs/multipart/status", url.Values{"upload_id": {"yunbo-capability-probe"}}, nil, nil, 0, &snapshot)
	var apiErr *APIError
	c.UploadResume = err == nil || (errors.As(err, &apiErr) && apiErr.Code == 404)
	if !c.UploadResume {
		c.UploadResumeReason = "当前服务器未提供分片续传接口，普通上传不能断点继续"
	} else {
		c.UploadResumeReason = ""
		var settings map[string]json.RawMessage
		if err := c.call(ctx, "GET", "/api/public/settings", nil, &settings); err == nil {
			enabled := string(settings["multipart_enabled"])
			if enabled == "false" || enabled == `"false"` || enabled == `"0"` {
				c.UploadResume = false
				c.UploadResumeReason = "服务器已关闭分片上传，普通上传不能断点继续"
			}
		}
	}
}

func (c *Client) MultipartStatus(ctx context.Context, id, remote string, size int64) (MultipartSnapshot, error) {
	query := url.Values{}
	if id != "" {
		query.Set("upload_id", id)
	} else {
		query.Set("path", CleanPath(remote))
		query.Set("size", strconv.FormatInt(size, 10))
	}
	var result MultipartSnapshot
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := c.multipartCall(ctx, "GET", "/api/fs/multipart/status", query, nil, nil, 0, &result)
	return result, err
}

func (c *Client) MultipartInit(ctx context.Context, remote string, size int64, modified time.Time, sha256 string) (MultipartSnapshot, error) {
	existing, err := c.MultipartStatus(ctx, "", remote, size)
	var apiErr *APIError
	if err == nil && existing.State == "receiving" {
		return MultipartSnapshot{}, errors.New("该目标已有分片上传会话，请继续原任务或等待其结束")
	}
	if err != nil && !(errors.As(err, &apiErr) && apiErr.Code == 404) {
		return MultipartSnapshot{}, err
	}
	headers := map[string]string{"File-Path": url.PathEscape(CleanPath(remote)), "X-File-Size": strconv.FormatInt(size, 10), "X-Chunk-Size": strconv.Itoa(4 << 20), "X-File-Sha256": sha256, "Last-Modified": strconv.FormatInt(modified.UnixMilli(), 10), "Content-Type": "application/octet-stream", "Overwrite": "false"}
	var result MultipartSnapshot
	err = c.multipartCall(ctx, "POST", "/api/fs/multipart/init", nil, headers, nil, 0, &result)
	if errors.As(err, &apiErr) && apiErr.Code == 403 && strings.Contains(strings.ToLower(apiErr.Message), "multipart") && strings.Contains(strings.ToLower(apiErr.Message), "disabled") {
		return result, ErrMultipartUnavailable
	}
	return result, err
}

func (c *Client) MultipartChunk(ctx context.Context, id string, index int, reader io.Reader, length int64, progress Progress, base, total int64) (MultipartSnapshot, error) {
	var result MultipartSnapshot
	if progress != nil {
		reader = &progressReader{reader: reader, done: base, total: total, progress: progress}
	}
	err := c.multipartCall(ctx, "PUT", "/api/fs/multipart/chunk", nil, map[string]string{"X-Upload-Id": id, "X-Chunk-Index": strconv.Itoa(index), "Content-Type": "application/octet-stream"}, reader, length, &result)
	return result, err
}

func (c *Client) MultipartComplete(ctx context.Context, id string) (MultipartSnapshot, error) {
	var result MultipartSnapshot
	err := c.multipartCall(ctx, "POST", "/api/fs/multipart/complete", nil, map[string]string{"X-Upload-Id": id}, nil, 0, &result)
	return result, err
}

func (c *Client) MultipartAbort(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.multipartCall(ctx, "POST", "/api/fs/multipart/abort", nil, map[string]string{"X-Upload-Id": id}, nil, 0, nil)
}
