package io

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

var LogDownload = base.NewLogCategory("Download")

type DownloadMode byte

const (
	DOWNLOAD_DEFAULT DownloadMode = iota
	DOWNLOAD_REDIRECT
)

func (x *DownloadMode) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x DownloadMode) String() string {
	switch x {
	case DOWNLOAD_DEFAULT:
		return "DOWNLOAD_DEFAULT"
	case DOWNLOAD_REDIRECT:
		return "DOWNLOAD_REDIRECT"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}

func BuildDownloader(uri string, dst utils.Filename, mode DownloadMode) utils.BuildFactoryTyped[*Downloader] {
	parsedUrl, err := url.Parse(uri)
	if err != nil {
		base.LogFatal("download: %v", err)
	}
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (Downloader, error) {
		return Downloader{
			Source:      *parsedUrl,
			Destination: dst,
			Mode:        mode,
		}, nil
	})
}

type Downloader struct {
	Source      url.URL
	Destination utils.Filename
	Mode        DownloadMode
}

func (dl *Downloader) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Download", dl.Destination.String())
}
func (dl *Downloader) Build(bc utils.BuildContext) error {
	var err error
	var written int64
	switch dl.Mode {
	case DOWNLOAD_DEFAULT:
		written, err = DownloadFile(dl.Destination, dl.Source)
	case DOWNLOAD_REDIRECT:
		written, err = DownloadHttpRedirect(dl.Destination, dl.Source)
	}

	if err == nil {
		err = bc.OutputFile(dl.Destination)
	}
	if err == nil { // avoid re-downloading after each rebuild
		bc.Annotate(
			utils.AnnocateBuildCommentWith(base.SizeInBytes(written)),
			utils.AnnocateBuildTimestamp(utils.UFS.MTime(dl.Destination)))
	}
	return err
}
func (dl *Downloader) Serialize(ar base.Archive) {
	if ar.Flags().IsLoading() {
		var uri string
		ar.String(&uri)

		parsedUrl, err := url.Parse(uri)
		if err == nil {
			dl.Source = *parsedUrl
		} else {
			ar.OnError(err)
		}
	} else {
		uri := dl.Source.String()
		ar.String(&uri)
	}

	ar.Serializable(&dl.Destination)
	ar.Serializable(&dl.Mode)
}

type downloadCacheResult interface {
	ShouldCache() bool
	error
}
type invalidCacheItem struct {
	error
}
type nonCachableResponse struct {
	error
}

func (invalidCacheItem) ShouldCache() bool {
	return true
}
func (nonCachableResponse) ShouldCache() bool {
	return false
}

func downloadFromCache(resp *http.Response) (utils.Filename, downloadCacheResult) {
	var contentHash []string
	if contentHash = resp.Header.Values("Content-Md5"); contentHash == nil {
		contentHash = resp.Header.Values("X-Goog-Hash")
	}

	if contentHash != nil {
		uid, err := base.SerializeAnyFingerprint(func(ar base.Archive) error {
			for _, it := range contentHash {
				ar.String(&it)
			}
			return nil
		}, base.Fingerprint{})
		base.LogPanicIfFailed(LogDownload, err)

		inCache := utils.UFS.Transient.Folder("DownloadCache").File(fmt.Sprintf("%v.bin", uid))
		if info, err := inCache.Info(); info != nil && err == nil {
			var totalSize int
			totalSize, err = strconv.Atoi(resp.Header.Get("Content-Length"))
			if err != nil {
				return inCache, nonCachableResponse{err}
			}

			if totalSize != int(info.Size()) {
				return inCache, invalidCacheItem{fmt.Errorf("%v: size don't match (%v != %v)", inCache, totalSize, info.Size())}
			}

			return inCache, nil // cache hit
		} else {
			return inCache, invalidCacheItem{fmt.Errorf("%v: entry does not exist", inCache)}
		}
	}
	return utils.Filename{}, nonCachableResponse{fmt.Errorf("can't find content hash in http header")}
}

func DownloadFile(dst utils.Filename, src url.URL) (int64, error) {
	base.LogVerbose(LogDownload, "downloading url '%v' to '%v'...", src.String(), dst.String())

	var written int64
	cacheFile, shouldCache := utils.Filename{}, false
	err := utils.UFS.CreateFile(dst, func(w *os.File) error {
		client := http.Client{
			CheckRedirect: func(r *http.Request, _ []*http.Request) error {
				r.URL.Opaque = r.URL.Path
				return nil
			},
		}
		defer client.CloseIdleConnections()

		resp, err := client.Get(src.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var cacheResult downloadCacheResult
		cacheFile, cacheResult = downloadFromCache(resp)
		if cacheResult == nil { // cache hit
			base.LogDebug(LogDownload, "cache hit on '%v'", cacheFile)

			return utils.UFS.OpenFile(cacheFile, func(r *os.File) (err error) {
				info, err := r.Stat()
				if err == nil {
					err = base.SetMTime(w, info.ModTime()) // keep mtime consistent
				}
				if err != nil {
					return err
				}

				err = base.CopyWithProgress(dst.Basename, info.Size(), w, r)
				written += info.Size()
				return
			})

		} else { // cache miss
			shouldCache = cacheResult.ShouldCache() // cachable ?

			written, err = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
			if err == nil {
				err = base.CopyWithProgress(dst.Basename, int64(written), w, resp.Body)
			}
		}

		return err
	})

	if err == nil && shouldCache {
		base.LogDebug(LogDownload, "cache store in '%v'", cacheFile)
		if err := utils.UFS.Copy(dst, cacheFile); err != nil {
			base.LogWarning(LogDownload, "failed to cache download with %v", err)
		}
	}

	return written, err
}

var re_metaRefreshRedirect = regexp.MustCompile(`(?i)<meta.*http-equiv="refresh".*content=".*url=(.*)".*?/>`)

func DownloadHttpRedirect(dst utils.Filename, src url.URL) (int64, error) {
	base.LogVerbose(LogDownload, "download http redirect '%v' to '%v'...", src.String(), dst.String())

	client := http.Client{
		CheckRedirect: func(r *http.Request, _ []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	defer client.CloseIdleConnections()

	resp, err := client.Get(src.String())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	parse := base.TransientBuffer.Allocate()
	defer base.TransientBuffer.Release(parse)

	_, err = io.Copy(parse, resp.Body)

	if err == nil {
		match := re_metaRefreshRedirect.FindSubmatch(parse.Bytes())
		if len(match) > 1 {
			var url *url.URL
			if url, err = url.Parse(base.UnsafeStringFromBytes(match[1])); err == nil {
				return DownloadFile(dst, *url)
			}
		} else {
			err = fmt.Errorf("http: could not find html refresh meta")
		}
	}

	return 0, err
}
