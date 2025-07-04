package io

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
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

func BuildDownloader(url base.Url, dst utils.Directory, mode DownloadMode) utils.BuildFactoryTyped[*Downloader] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (Downloader, error) {
		return Downloader{
			Source:      url,
			DownloadDir: dst,
			Mode:        mode,
		}, nil
	})
}

func BuildDownloaderFromUrl(uri string, dst utils.Directory, mode DownloadMode) utils.BuildFactoryTyped[*Downloader] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (Downloader, error) {
		var url base.Url
		err := url.Set(uri)
		return Downloader{
			Source:      url,
			DownloadDir: dst,
			Mode:        mode,
		}, err
	})
}

type Downloader struct {
	Source      base.Url
	Destination utils.Filename
	DownloadDir utils.Directory
	Mode        DownloadMode
}

func (dl *Downloader) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Download", dl.Source.String(), "->", dl.DownloadDir.String())
}
func (dl *Downloader) Build(bc utils.BuildContext) error {
	client := http.Client{
		CheckRedirect: func(r *http.Request, _ []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	defer client.CloseIdleConnections()

	var err error
	var written int64
	switch dl.Mode {
	case DOWNLOAD_DEFAULT:
		dl.Destination = dl.DownloadDir.File(path.Base(dl.Source.Path))
		written, err = DownloadFile(bc, &client, dl.Destination, dl.Source)
	case DOWNLOAD_REDIRECT:
		dl.Destination, written, err = DownloadHttpRedirect(bc, &client, dl.DownloadDir, dl.Source)
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
	ar.Serializable(&dl.Source)
	ar.Serializable(&dl.Destination)
	ar.Serializable(&dl.DownloadDir)
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

var errMissingHTTPHeaderContentLength = errors.New("missing HTTP header `Content-Length`, not a file URL?")

func downloadFromCache(resp *http.Response) (utils.Filename, downloadCacheResult) {
	var contentHash []string
	if contentHash = resp.Header.Values("Content-Md5"); contentHash == nil {
		if contentHash = resp.Header.Values("X-Goog-Hash"); contentHash == nil {
			contentHash = resp.Header.Values("X-Ms-Blob-Content-Md5")
		}
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
			var contentLength int64
			if contentLengthStr := resp.Header.Get("Content-Length"); len(contentLengthStr) > 0 {
				if contentLength, err = strconv.ParseInt(contentLengthStr, 10, 64); err != nil {
					return inCache, nonCachableResponse{err}
				}
			} else {
				return inCache, nonCachableResponse{errMissingHTTPHeaderContentLength}
			}

			if contentLength != info.Size() {
				return inCache, invalidCacheItem{fmt.Errorf("%v: size don't match (%v != %v)", inCache, contentLength, info.Size())}
			}

			return inCache, nil // cache hit
		} else {
			return inCache, invalidCacheItem{fmt.Errorf("%v: entry does not exist", inCache)}
		}
	}
	return utils.Filename{}, nonCachableResponse{fmt.Errorf("can't find content hash in http header")}
}

func DownloadFile(ctx context.Context, client *http.Client, dst utils.Filename, src base.Url) (int64, error) {
	base.LogVerbose(LogDownload, "downloading url '%v' to '%v'...", src.String(), dst.String())

	// Create a new HTTP request with context
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, src.String(), nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	cacheFile, cacheResult := downloadFromCache(resp)

	if cacheResult == nil { // cache hit
		base.LogDebug(LogDownload, "cache hit on '%v'", cacheFile)

		cacheInfo, err := cacheFile.Info()
		if err != nil {
			return 0, err
		}

		if dstInfo, err := dst.Info(); err == nil && cacheInfo.ModTime().UTC().Equal(dstInfo.ModTime().UTC()) && cacheInfo.Size() == dstInfo.Size() {
			// destination size and mtime are already matching, skip copy file
			base.LogVerbose(LogDownload, "skipping copy of %q, since mod time and size perfectly match", cacheFile)
			return dstInfo.Size(), nil
		}

		err = utils.UFS.Copy(ctx, cacheFile, dst, false)
		return cacheInfo.Size(), err

	} else { // cache miss
		var contentLength int64
		if contentLengthStr := resp.Header.Get("Content-Length"); len(contentLengthStr) > 0 {
			if contentLength, err = strconv.ParseInt(contentLengthStr, 10, 64); err != nil {
				return 0, err
			}
		} else {
			return 0, errMissingHTTPHeaderContentLength
		}

		err = utils.UFS.CreateFile(dst, func(w *os.File) error {
			return base.CopyWithProgress(ctx, utils.MakeShortUserFriendlyPath(src).String(), contentLength, w, resp.Body)
		})

		if err == nil && cacheResult.ShouldCache() {
			base.LogDebug(LogDownload, "cache store in '%v'", cacheFile)
			if err := utils.UFS.Copy(ctx, dst, cacheFile, false); err != nil {
				base.LogWarning(LogDownload, "failed to cache download with %v", err)
			}
		}

		return contentLength, err
	}
}

var re_metaRefreshRedirect = regexp.MustCompile(`(?i)<meta.*http-equiv="refresh".*content=".*url=(.*)".*?/>`)

func DownloadHttpRedirect(ctx context.Context, client *http.Client, dst utils.Directory, src base.Url) (utils.Filename, int64, error) {
	base.LogVerbose(LogDownload, "download http redirect '%v' to '%v'...", src.String(), dst.String())

	// Create a new HTTP request with context
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, src.String(), nil)
	if err != nil {
		return utils.Filename{}, 0, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return utils.Filename{}, 0, err
	}
	defer resp.Body.Close()

	parse := base.TransientBuffer.Allocate()
	defer base.TransientBuffer.Release(parse)

	_, err = base.TransientIoCopy(ctx, parse, resp.Body, base.TransientPage4KiB, false)

	if err == nil {
		match := re_metaRefreshRedirect.FindSubmatch(parse.Bytes())
		if len(match) > 1 {
			var url *url.URL
			if url, err = url.Parse(base.UnsafeStringFromBytes(match[1])); err == nil {
				localFile := dst.File(path.Base(url.Path))
				written, err := DownloadFile(ctx, client, localFile, base.Url{URL: url})
				return localFile, written, err
			}
		} else {
			err = fmt.Errorf("http: could not find html refresh meta")
		}
	}

	return utils.Filename{}, 0, err
}
