package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/winezer0/okserver/internal/progress"
	"github.com/winezer0/okserver/internal/transfer"
)

// Options configures transport independently of command-line parsing.
type Options struct {
	DownloadConcurrency             int
	URL, Data, Output, Auth, CACert string
	Resume, Insecure                bool
	Timeout                         time.Duration
}

// Execute runs one transfer using the public HTTP protocol.
func Execute(options Options) error {
	return ExecuteContext(context.Background(), options)
}

func ExecuteContext(ctx context.Context, options Options) error {
	client := &fileClient{ctx: ctx, data: options.Data, output: options.Output, point: options.Resume, pBar: progress.NewProgress(), downloadConcurrency: options.DownloadConcurrency}
	if client.downloadConcurrency == 0 {
		client.downloadConcurrency = 4
	}
	if client.downloadConcurrency < 1 || client.downloadConcurrency > 16 {
		return errors.New("download concurrency must be 1..16")
	}
	defer client.pBar.Shutdown()
	var err error
	if options.Auth != "" {
		var ok bool
		client.user, client.pass, ok = strings.Cut(options.Auth, ":")
		if !ok {
			return errors.New("invalid auth header")
		}
	}

	if client.httpUrl = options.URL; client.httpUrl == "" {
		return errors.New("url is null")
	}

	// 最小时间和http.DefaultTransport保持一致
	limitTime := func(d, limit time.Duration) time.Duration {
		if d < limit {
			return limit
		}
		return d
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   limitTime(options.Timeout/2, 30*time.Second),
			KeepAlive: limitTime(options.Timeout/2, 30*time.Second),
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       limitTime(options.Timeout*2, 90*time.Second),
		TLSHandshakeTimeout:   limitTime(options.Timeout/6, 10*time.Second),
		ExpectContinueTimeout: limitTime(options.Timeout/60, time.Second),
	}

	// 证书配置,可以忽略证书,也可以携带ca.crt证书
	if options.Insecure || options.CACert != "" {
		var root *x509.CertPool
		if options.CACert != "" {
			pemCerts, err := os.ReadFile(options.CACert)
			if err != nil {
				return err
			}

			root = x509.NewCertPool()
			root.AppendCertsFromPEM(pemCerts)
		}

		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: options.Insecure,
			RootCAs:            root,
		}
	}
	defer transport.CloseIdleConnections()
	client.client = &http.Client{Timeout: options.Timeout, Transport: transport}

	client.buf = transfer.GetBuffer()
	defer transfer.PutBuffer(client.buf)

	if client.data != "" {
		err = client.post()
	} else {
		if client.downloadConcurrency > 1 && client.output != "-" {
			err = client.parallelDownload()
		} else {
			err = client.get()
		}
	}
	if err == nil {
		client.pBar.Wait()
	}
	return redactTransferError(err)
}

func redactTransferError(err error) error {
	var requestErr *url.Error
	if !errors.As(err, &requestErr) {
		return err
	}
	copy := *requestErr
	if address, parseErr := url.Parse(copy.URL); parseErr == nil {
		address.User, address.RawQuery, address.Fragment = nil, "", ""
		copy.URL = address.String()
	} else {
		copy.URL = "[redacted URL]"
	}
	return &copy
}

type fileClient struct {
	uploadToken         string
	downloadConcurrency int
	ctx                 context.Context
	client              *http.Client
	pBar                *mpb.Progress
	httpUrl             string
	data                string
	output              string
	user                string
	pass                string
	buf                 []byte
	point               bool
}

func (fc *fileClient) newRequest(method, url string, body io.Reader, hd http.Header) (*http.Request, error) {
	req, err := http.NewRequestWithContext(fc.context(), method, url, body)
	if err != nil {
		return nil, err
	}

	if hd != nil {
		req.Header = hd
	}

	if fc.user != "" && fc.pass != "" {
		req.SetBasicAuth(fc.user, fc.pass)
	}
	if fc.uploadToken != "" && strings.HasPrefix(req.URL.Path, "/upload/") {
		req.Header.Set("Upload-Token", fc.uploadToken)
	}
	return req, nil
}

func (fc *fileClient) post() error {
	source, ok := strings.CutPrefix(fc.data, "@")
	if !ok || source == "" {
		return errors.New("file upload requires -d @file")
	}
	return fc.serialUpload(source)
}
