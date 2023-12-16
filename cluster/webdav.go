package cluster

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/shirou/gopsutil/disk"
	"golang.org/x/net/webdav"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

// #TODO: use TLS

/***************************************
 * Webdav server
 ***************************************/

var LogWebdav = base.NewLogCategory("Webdav")

type WebdavPartition struct {
	Mountpoint utils.Directory
	Handler    webdav.Handler
}

func newWebdavPartition(part disk.PartitionStat, logger func(r *http.Request, err error)) WebdavPartition {
	mountPoint := utils.MakeDirectory(fmt.Sprint(part.Mountpoint, string(utils.OSPathSeparator)))
	return WebdavPartition{
		Mountpoint: mountPoint,
		Handler: webdav.Handler{
			Prefix:     fmt.Sprintf("/dav/%s", base.SanitizeIdentifier(part.Mountpoint)),
			FileSystem: webdav.Dir(part.Mountpoint),
			LockSystem: webdav.NewMemLS(),
			Logger:     logger,
		},
	}
}
func (x *WebdavPartition) GetEndpoint(srv *WebdavServer) string {
	return fmt.Sprint("http://", srv.GetAddress().String(), x.Handler.Prefix)
}
func (x *WebdavPartition) GetPattern() string {
	// must end by a slash to be considered as prefix pattern matching by ServeMux.match()
	return fmt.Sprint(x.Handler.Prefix, `/`)
}

type WebdavServer struct {
	addr    net.TCPAddr
	async   base.Future[int]
	context context.Context
	server  http.Server

	muxer      *http.ServeMux
	partitions []WebdavPartition
}

func NewWebdavServer(ctx context.Context) (result *WebdavServer) {
	result = &WebdavServer{
		muxer: http.NewServeMux(),
	}

	partitions, err := disk.PartitionsWithContext(ctx, false)
	base.LogPanicIfFailed(LogWebdav, err)

	logger := func(r *http.Request, err error) {
		if err != nil {
			base.LogWarning(LogWebdav, "[%s] %q: %v", r.Method, r.URL, err)
		} else {
			base.LogTrace(LogWebdav, "[%s] %q", r.Method, r.URL)
		}
	}

	result.partitions = make([]WebdavPartition, len(partitions))

	for i, part := range partitions {
		result.partitions[i] = newWebdavPartition(part, logger)

		base.LogVeryVerbose(LogWebdav, "mounting %q as %q",
			result.partitions[i].Mountpoint,
			result.partitions[i].Handler.Prefix)

		result.muxer.Handle(result.partitions[i].GetPattern(), &result.partitions[i].Handler)
	}

	return
}
func (x *WebdavServer) GetScheme() string    { return "http" } // #TODO: https self-signed?
func (x *WebdavServer) GetAddress() net.Addr { return &x.addr }
func (x *WebdavServer) Start(ctx context.Context, host net.IP, port string) error {
	base.AssertIn(x.async, nil)

	timeout := GetClusterFlags().GetTimeoutDuration()

	x.context = ctx
	x.server = http.Server{
		Addr:    net.JoinHostPort("", port),
		Handler: webdavCompressionHandler{x.muxer},
		BaseContext: func(net.Listener) context.Context {
			return x.context
		},
		ReadHeaderTimeout: timeout,
		WriteTimeout:      timeout,
		// TLSConfig:         generateServerTLSConfig(), // override certFile/keyFile parameters of ServeTLS() bellow
	}

	if base.DEBUG_ENABLED {
		x.server.ErrorLog = log.New(base.LogForwardWriter{}, "webdav", 0)
	}

	listener, err := net.Listen("tcp", x.server.Addr)
	if err != nil {
		return err
	}

	tcp := listener.Addr().(*net.TCPAddr)
	x.addr = *tcp
	x.addr.IP = host

	x.async = base.MakeAsyncFuture[int](func() (int, error) {
		defer x.server.Close()
		defer listener.Close()

		base.LogVerbose(LogWebdav, "webdav server starts listening on %q", listener.Addr())

		for _, part := range x.partitions {
			base.LogVerbose(LogWebdav, "mounted %q as %q", part.Mountpoint, part.GetEndpoint(x))
		}

		base.FlushLog()

		return 0, x.server.Serve(listener)
		// return 0, x.server.ServeTLS(listener, "", "")
	})

	return nil
}
func (x *WebdavServer) Close() (err error) {
	base.AssertNotIn(&x.async, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	serverErr := x.server.Shutdown(ctx)
	err = x.async.Join().Failure()
	if err == nil {
		err = serverErr
	}

	x.addr = net.TCPAddr{}
	x.async = nil
	x.context = nil
	x.server = http.Server{}
	return
}

/***************************************
 * Webdav HTTP compression
 ***************************************/

type webdavCompressionHandler struct {
	muxer *http.ServeMux
}

func (x webdavCompressionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if gzip or deflate is supported
	acceptEncoding := r.Header.Get("Accept-Encoding")
	te := r.Header.Get("TE")

	if strings.Contains(acceptEncoding, "gzip") || strings.Contains(te, "gzip") {
		// Compression is supported
		// Handle the request with compression
		if err := x.gzip(w, r); err == nil {
			return
		}

	}

	if strings.Contains(acceptEncoding, "deflate") || strings.Contains(te, "deflate") {
		// Compression is supported (TE header)
		// Handle the request with compression
		if err := x.deflate(w, r); err == nil {
			return
		}
	}

	// Compression is not supported (or failed)
	// Handle the request without compression
	x.muxer.ServeHTTP(w, r)
}

func (x webdavCompressionHandler) deflate(w http.ResponseWriter, r *http.Request) error {
	// Create a deflate response writer
	dw, err := flate.NewWriter(w, flate.DefaultCompression)
	if err != nil {
		return err
	}

	defer dw.Close()

	// Set the appropriate headers
	w.Header().Set("Content-Encoding", "deflate")
	w.Header().Set("Vary", "Accept-Encoding")

	// Wrap the original response writer with gzip writer
	dzw := compressedResponseWriter{ResponseWriter: w, compressed: dw}
	x.muxer.ServeHTTP(&dzw, r)

	return nil
}

func (x webdavCompressionHandler) gzip(w http.ResponseWriter, r *http.Request) error {
	// Create a gzip response writer
	gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}

	defer gz.Close()

	// Set the appropriate headers
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Vary", "Accept-Encoding")

	// Wrap the original response writer with gzip writer
	gzw := compressedResponseWriter{ResponseWriter: w, compressed: gz}
	x.muxer.ServeHTTP(&gzw, r)

	return nil
}

/***************************************
 * HTTP compressed response
 ***************************************/

type compressedResponseWriter struct {
	http.ResponseWriter
	compressed io.WriteCloser
}

func (x *compressedResponseWriter) Write(b []byte) (int, error) {
	return x.compressed.Write(b)
}
func (x *compressedResponseWriter) Close() error {
	return x.compressed.Close()
}
