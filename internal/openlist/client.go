// Package openlist implements the public OpenList HTTP API without depending on
// or embedding the OpenList server.
package openlist

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type File struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified string `json:"modified"`
	Sign     string `json:"sign"`
	Type     int    `json:"type"`
}

type Listing struct {
	Content            []File `json:"content"`
	Total              int    `json:"total"`
	Write              bool   `json:"write"`
	WriteContentBypass bool   `json:"write_content_bypass"`
}

type User struct {
	Username   string `json:"username"`
	BasePath   string `json:"base_path"`
	Role       int    `json:"role"`
	Permission int64  `json:"permission"`
	Disabled   bool   `json:"disabled"`
}

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	switch e.Code {
	case 401:
		return "登录已失效或账号密码错误：" + e.Message
	case 402:
		return "需要有效的两步验证码：" + e.Message
	case 403:
		return "没有访问权限或目录密码错误：" + e.Message
	case 429:
		return "请求过于频繁，请稍后再试：" + e.Message
	}
	return fmt.Sprintf("OpenList (%d)：%s", e.Code, e.Message)
}

type Client struct {
	base               *url.URL
	token              string
	User               User
	api                *http.Client
	transfer           *http.Client
	UploadResume       bool
	UploadResumeReason string
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("服务器返回 HTTP %d，请检查地址、反向代理或服务状态", e.Status)
}

func New(address, token string) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("请输入完整的 http:// 或 https:// OpenList 服务地址")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("服务地址不能包含账号、查询参数或片段；子路径部署可填写 https://域名/openlist")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 2 * time.Minute
	return &Client{base: u, token: strings.TrimSpace(token),
		api: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // Never redirect credential-bearing API calls.
		}},
		transfer: &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("下载重定向次数过多")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("下载重定向协议无效")
			}
			// Signed /d links do not need the account token, even on the first hop.
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			return nil
		}},
	}, nil
}

func (c *Client) Address() string { return c.base.String() }

func (c *Client) endpoint(suffix string) string {
	u := *c.base
	u.Path += suffix
	u.RawPath = ""
	return u.String()
}

func requestError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("网络请求失败：%w", uerr.Err)
	}
	return err
}

func decodeResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Status: resp.StatusCode}
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&envelope); err != nil {
		return errors.New("服务器没有返回有效的 OpenList API 数据，请检查是否填写了服务根地址")
	}
	if envelope.Code != 200 {
		return &APIError{envelope.Code, envelope.Message}
	}
	if out != nil {
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			return errors.New("OpenList 返回了空数据")
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("读取 OpenList 数据失败：%w", err)
		}
	}
	return nil
}

func (c *Client) call(ctx context.Context, method, endpoint string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(endpoint), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
	resp, err := c.api.Do(req)
	if err != nil {
		return requestError(err)
	}
	return decodeResponse(resp, out)
}

func (c *Client) Login(ctx context.Context, username, password, otp string) error {
	h := sha256.Sum256([]byte(password + "-https://github.com/alist-org/alist"))
	var result struct {
		Token string `json:"token"`
	}
	if err := c.call(ctx, "POST", "/api/auth/login/hash", map[string]string{
		"username": username, "password": hex.EncodeToString(h[:]), "otp_code": otp,
	}, &result); err != nil {
		return err
	}
	if result.Token == "" {
		return errors.New("登录成功但服务器没有返回令牌")
	}
	c.token = result.Token
	return nil
}

func (c *Client) LoadUser(ctx context.Context) error {
	if err := c.call(ctx, "GET", "/api/me", nil, &c.User); err != nil {
		return err
	}
	if c.User.Disabled {
		return errors.New("此账号或访客访问已被服务器禁用")
	}
	return nil
}

func CleanPath(p string) string { return path.Clean("/" + strings.TrimLeft(p, "/")) }

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") {
		return errors.New("名称不能为空，也不能包含斜杠、换行或相对路径")
	}
	return nil
}

func (c *Client) List(ctx context.Context, p, password string, page int) (Listing, error) {
	if page < 1 {
		page = 1
	}
	var result Listing
	err := c.call(ctx, "POST", "/api/fs/list", map[string]any{
		"path": CleanPath(p), "password": password, "page": page, "per_page": 100, "refresh": false,
	}, &result)
	if result.Content == nil {
		result.Content = []File{}
	}
	return result, err
}

func (c *Client) Mkdir(ctx context.Context, parent, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	return c.call(ctx, "POST", "/api/fs/mkdir", map[string]string{"path": path.Join(CleanPath(parent), name)}, nil)
}

func (c *Client) Rename(ctx context.Context, p, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if CleanPath(p) == "/" {
		return errors.New("不能重命名根目录")
	}
	return c.call(ctx, "POST", "/api/fs/rename", map[string]any{"path": CleanPath(p), "name": name, "overwrite": false}, nil)
}

func (c *Client) Remove(ctx context.Context, dir string, names []string) error {
	if len(names) == 0 {
		return errors.New("请选择要删除的文件")
	}
	for _, name := range names {
		if err := ValidateName(name); err != nil {
			return err
		}
	}
	return c.call(ctx, "POST", "/api/fs/remove", map[string]any{"dir": CleanPath(dir), "names": names}, nil)
}

func (c *Client) DownloadLink(ctx context.Context, p, password string) (string, File, error) {
	var result struct{ File }
	if err := c.call(ctx, "POST", "/api/fs/get", map[string]string{"path": CleanPath(p), "password": password}, &result); err != nil {
		return "", File{}, err
	}
	if result.IsDir {
		return "", File{}, errors.New("首版仅支持下载文件，请进入文件夹后选择文件")
	}
	// /api/fs/* uses paths relative to the account; /d/* uses storage paths.
	u, _ := url.Parse(c.endpoint("/d" + path.Join("/", c.User.BasePath, CleanPath(p))))
	q := u.Query()
	if result.Sign != "" {
		q.Set("sign", result.Sign)
	}
	u.RawQuery = q.Encode()
	return u.String(), result.File, nil
}

type Progress func(done, total int64)

type progressReader struct {
	reader      io.Reader
	done, total int64
	progress    Progress
}

func (r *progressReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	r.done += int64(n)
	if r.progress != nil {
		r.progress(r.done, r.total)
	}
	return n, err
}

func (c *Client) Upload(ctx context.Context, local, remote string, progress Progress) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	return c.UploadFile(ctx, f, remote, progress)
}

func (c *Client) UploadFile(ctx context.Context, f *os.File, remote string, progress Progress) error {
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return errors.New("只能上传普通文件")
	}
	reader := &progressReader{reader: f, total: stat.Size(), progress: progress}
	req, err := http.NewRequestWithContext(ctx, "PUT", c.endpoint("/api/fs/put"), reader)
	if err != nil {
		return err
	}
	req.ContentLength = stat.Size()
	if stat.Size() == 0 {
		req.Body = http.NoBody
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("File-Path", url.PathEscape(CleanPath(remote)))
	req.Header.Set("As-Task", "false")
	req.Header.Set("Overwrite", "false")
	req.Header.Set("Last-Modified", fmt.Sprint(stat.ModTime().UnixMilli()))
	resp, err := c.api.Do(req)
	if err != nil {
		return requestError(err)
	}
	return decodeResponse(resp, nil)
}

// Download writes a stream to a caller-owned temporary file. The caller only
// publishes that file after this function has verified the complete response.
func (c *Client) Download(ctx context.Context, remote, password string, dst io.Writer, progress Progress) error {
	link, file, err := c.DownloadLink(ctx, remote, password)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return err
	}
	resp, err := c.transfer.Do(req)
	if err != nil {
		return requestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total < 0 {
		total = file.Size
	}
	reader := &progressReader{reader: resp.Body, total: total, progress: progress}
	n, err := io.Copy(dst, reader)
	if err != nil {
		return err
	}
	if total > 0 && n != total {
		return fmt.Errorf("下载不完整：应接收 %d 字节，实际 %d 字节", total, n)
	}
	return nil
}
