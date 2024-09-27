package cluster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"

	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Message Loop
 ***************************************/

func MessageLoop(tunnel *Tunnel, ctx context.Context, timeout time.Duration, inbox ...MessageBody) (err error) {
	rw := newMessageReadWriter(tunnel)

	pingTicker := time.NewTicker(timeout)
	defer pingTicker.Stop()

	err = base.Recover(func() (err error) {
		for _, body := range inbox {
			if err := rw.Write(body); err != nil {
				return err
			}
		}

		defer func() {
			if err != nil {
				base.LogError(&rw.category, "close with failure: %v", err)
			}
		}()

		for rw.Alive() {
			var msg MessageType

			select {
			case <-ctx.Done():
				base.LogWarning(&rw.category, "closed by context: %v", ctx.Err())
				return ctx.Err()

			case <-pingTicker.C:
				base.LogTrace(&rw.category, "ping (latency=%v)", rw.tunnel.ping)

				if tunnel.TimeSinceLastWrite() > timeout/2 {
					if err = WriteMessage(&rw, NewMessagePing()); err != nil {
						if rw.Retry(err) {
							err = nil
							continue
						}
						return err
					}
				}
				continue

			default:
				msg, err = rw.Peek()
				if err != nil {
					if rw.Retry(err) {
						err = nil
						continue
					}
					return
				}
			}

			if err = rw.Read(msg); err != nil {
				break
			}
		}

		return
	})
	return
}

/***************************************
 * Message Read/Writer
 ***************************************/

type MessageTunnel interface {
	Alive() bool
	ReadyForWork() bool
	Retry(error) bool
	Close(error) error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Events() *TunnelEvents
}

type MessageReader interface {
	Peek() (MessageType, error)
	Read(header MessageType) error
	MessageTunnel
}

type MessageWriter interface {
	Flush() error // for MessageBatch
	Write(body MessageBody) error
	UpdateLatency(remoteUTC time.Time)
	MessageTunnel
}

func WriteMessage[T any, M interface {
	*T
	MessageBody
}](wr MessageWriter, msg T) error {
	return wr.Write(M(&msg))
}

type messageReadWriter struct {
	tunnel                *Tunnel
	localAddr, remoteAddr net.Addr

	rd base.ArchiveBinaryReader
	wr base.ArchiveBinaryWriter

	category base.LogCategory
	batching map[uintptr]MessageBatch

	retryCount, retryMaxCount int
}

func newMessageReadWriter(tunnel *Tunnel) (result messageReadWriter) {
	result.tunnel = tunnel
	result.localAddr, result.remoteAddr = tunnel.conn.LocalAddr(), tunnel.conn.RemoteAddr()
	result.category = base.MakeLogCategory(result.remoteAddr.String())

	result.rd = base.NewArchiveBinaryReader(internal_io.NewObservableReader(base.NewCompressedReader(tunnel, tunnel.Compression.Options), tunnel.Cluster.UncompressRead), base.AR_TOLERANT)

	result.wr = base.NewArchiveBinaryWriter(internal_io.NewObservableWriter(base.NewCompressedWriter(tunnel, tunnel.Compression.Options), tunnel.Cluster.CompressWrite))
	result.wr.HandleErrors(func(err error) {
		base.LogPanic(&result.category, "caught archive write error: %v", err)
	})

	result.batching = make(map[uintptr]MessageBatch, 2)
	result.retryCount = 0
	result.retryMaxCount = GetClusterFlags().RetryCount.Get()
	return
}

func (x *messageReadWriter) Alive() bool {
	return x.tunnel != nil
}
func (x *messageReadWriter) ReadyForWork() bool {
	return x.tunnel.OnReadyForWork()
}
func (x *messageReadWriter) Retry(err error) bool {
	if os.IsTimeout(err) {
		x.retryCount++
		base.LogWarning(&x.category, "should retry (%d/%d): %v", x.retryCount, x.retryMaxCount, err)
		return x.retryCount < x.retryMaxCount
	}
	return false
}
func (x *messageReadWriter) Close(err error) error {
	if x.tunnel != nil {
		err = x.Flush()

		base.LogTrace(&x.category, "closed with: %v", err)

		errWr, errRd := x.wr.Close(), x.rd.Close()
		if errWr != nil {
			err = errWr
		}
		if errRd != nil {
			err = errRd
		}

		x.tunnel = nil
	}
	return err
}

func (x *messageReadWriter) LocalAddr() net.Addr   { return x.localAddr }
func (x *messageReadWriter) RemoteAddr() net.Addr  { return x.remoteAddr }
func (x *messageReadWriter) Events() *TunnelEvents { return &x.tunnel.TunnelEvents }

func (x *messageReadWriter) Peek() (msg MessageType, err error) {
	x.rd.Reset(x.tunnel)
	x.rd.HandleErrors(func(err error) {
		base.LogWarning(&x.category, "caught error while reading message header: %v (retry=%d/%d)", err, x.retryCount, x.retryMaxCount)
	})

	x.rd.Serializable(&msg)
	return msg, x.rd.Error()
}
func (x *messageReadWriter) Read(header MessageType) error {
	x.retryCount = 0 // reset retry count (assuming a message header was already serialized)

	x.rd.HandleErrors(func(err error) {
		base.LogPanic(&x.category, "caught archive serialization error: %v", err)
	})

	return header.Body(func(body MessageBody) (err error) {
		x.rd.Serializable(body)
		if err = x.rd.Error(); err == nil {
			base.LogTrace(&x.category, "read [%v:%#v] %v", header, header, base.PrettyPrint(body))
			err = body.Accept(x)
		} else {
			base.LogError(&x.category, "failed to parse remote message: %v", err)
		}
		return
	})
}

func (x *messageReadWriter) UpdateLatency(remoteUTC time.Time) {
	x.tunnel.ping = time.Now().UTC().Sub(remoteUTC)
}
func (x *messageReadWriter) Flush() error {
	for typ, batched := range x.batching {
		delete(x.batching, typ)
		if err := x.writeMessageImmediate(batched); err != nil {
			return err
		}
	}
	return nil
}
func (x *messageReadWriter) Write(body MessageBody) error {
	// handle batchable messages, which are accumulated before actually sending
	if batched, ok := body.(MessageBatch); ok {
		typ := base.UnsafeTypeptr(batched)
		if bt, ok := x.batching[typ]; ok {
			if bt.Append(batched) {
				delete(x.batching, typ)
				return x.writeMessageImmediate(bt)
			}
		} else {
			x.batching[typ] = batched
		}
		return nil
	}

	return x.writeMessageImmediate(body)
}
func (x *messageReadWriter) writeMessageImmediate(body MessageBody) error {
	header := body.Header()
	base.LogTrace(&x.category, "write [%v:%#v] %v", header, header, base.PrettyPrint(body))

	if enableMessageCorpusForZStd {
		GetMessageCorpus().Add(body)
	}

	x.wr.Serializable(&header)
	x.wr.Serializable(body)

	err := x.wr.Error()

	// reset the writer to flush compression and actually write to the tunnel
	if er := x.wr.Reset(x.tunnel); er != nil && err == nil {
		err = nil
	}
	return err
}

/***************************************
 * Message Body
 ***************************************/

type MessageBody interface {
	Header() MessageType
	Accept(MessageWriter) error
	base.Serializable
}

type MessageBatch interface {
	Append(MessageBody) bool
	MessageBody
}

type timedMessageBody struct {
	Timestamp time.Time
}

func newTimedMessageBody() timedMessageBody {
	return timedMessageBody{
		Timestamp: time.Now().UTC(),
	}
}
func (x *timedMessageBody) Accept(wr MessageWriter) error {
	wr.UpdateLatency(x.Timestamp)
	return nil
}
func (x *timedMessageBody) Serialize(ar base.Archive) {
	ar.Time(&x.Timestamp)
}

type errorMessageBody struct {
	ErrCode RemoteErrorCode
	Message string
	timedMessageBody

	remoteErr error
}

func newErrorMessageBody(err error) errorMessageBody {
	errCode := REMOTE_NOERROR
	var errMessage string
	if err != nil {
		switch er := err.(type) {
		case RemoteTaskError:
			errCode = er.ErrorCode
			errMessage = er.Message
		default:
			errCode = REMOTE_ERR_PROCESS
			errMessage = err.Error()
		}
	}
	return errorMessageBody{
		ErrCode:          errCode,
		Message:          errMessage,
		timedMessageBody: newTimedMessageBody(),
	}
}
func (x *errorMessageBody) Err() error {
	return x.remoteErr
}
func (x *errorMessageBody) Accept(wr MessageWriter) error {
	wr.UpdateLatency(x.Timestamp)
	if x.ErrCode != REMOTE_NOERROR {
		x.remoteErr = NewRemoteTaskError(wr, x.ErrCode, x.Message)
	}
	return nil
}
func (x *errorMessageBody) Serialize(ar base.Archive) {
	x.timedMessageBody.Serialize(ar)
	ar.Int32((*int32)(&x.ErrCode))
	ar.String(&x.Message)
}

/***************************************
 * Remote Task Errors
 ***************************************/

type RemoteErrorCode int32

const (
	REMOTE_NOERROR      RemoteErrorCode = 0
	REMOTE_ERR_INTERNAL RemoteErrorCode = iota
	REMOTE_ERR_PROCESS
	REMOTE_ERR_REFUSED
	REMOTE_ERR_TIMEOUT
)

func (x RemoteErrorCode) String() string {
	switch x {
	case REMOTE_NOERROR:
		return "no error"
	case REMOTE_ERR_INTERNAL:
		return "remote internal error"
	case REMOTE_ERR_PROCESS:
		return "remote process failed"
	case REMOTE_ERR_REFUSED:
		return "remote refused task"
	case REMOTE_ERR_TIMEOUT:
		return "remote timeout"
	default:
		base.UnexpectedValuePanic(x, x)
		return ""
	}
}

type RemoteTaskError struct {
	Remote    net.Addr
	ErrorCode RemoteErrorCode
	Message   string
}

func NewRemoteTaskError(wr MessageWriter, code RemoteErrorCode, message string) RemoteTaskError {
	var remoteAddr net.Addr
	if wr != nil {
		remoteAddr = wr.RemoteAddr()
	}
	return RemoteTaskError{
		Remote:    remoteAddr,
		ErrorCode: code,
		Message:   message,
	}
}

func (x RemoteTaskError) Internal() bool { return x.ErrorCode == REMOTE_ERR_INTERNAL }
func (x RemoteTaskError) Process() bool  { return x.ErrorCode == REMOTE_ERR_PROCESS }
func (x RemoteTaskError) Refused() bool  { return x.ErrorCode == REMOTE_ERR_REFUSED }
func (x RemoteTaskError) Timeout() bool  { return x.ErrorCode == REMOTE_ERR_TIMEOUT }

func (x RemoteTaskError) Error() string {
	return fmt.Sprintf("%v: %s (%v)", x.ErrorCode, x.Message, x.Remote)
}

/***************************************
 * Message Ping/Pong
 ***************************************/

type MessagePing struct {
	timedMessageBody
}

func NewMessagePing() MessagePing {
	return MessagePing{newTimedMessageBody()}
}
func (x *MessagePing) Header() MessageType { return MSG_PING }
func (x *MessagePing) Accept(wr MessageWriter) error {
	if err := x.timedMessageBody.Accept(wr); err != nil {
		return err
	}
	return WriteMessage(wr, NewMessagePong())
}

type MessagePong struct {
	timedMessageBody
}

func NewMessagePong() MessagePong {
	return MessagePong{newTimedMessageBody()}
}
func (x *MessagePong) Header() MessageType { return MSG_PONG }

/***************************************
 * Message Goodbye (close tunnel)
 ***************************************/

type MessageGoodbye struct {
	timedMessageBody
}

func NewMessageGoodbye() MessageGoodbye {
	return MessageGoodbye{newTimedMessageBody()}
}
func (x *MessageGoodbye) Header() MessageType { return MSG_GOODBYE }
func (x *MessageGoodbye) Accept(wr MessageWriter) error {
	err := x.timedMessageBody.Accept(wr)
	return wr.Close(err)
}

/***************************************
 * Message Task
 ***************************************/

type MessageTaskDispatch struct {
	Executable      Filename
	Arguments       base.StringSet
	Environment     internal_io.ProcessEnvironment
	MountedPaths    []internal_io.MountedPath
	UseResponseFile bool
	WorkingDir      Directory

	timedMessageBody
}

func NewMessageTaskDispatch(executable Filename, arguments base.StringSet, workingDir Directory, env internal_io.ProcessEnvironment, mountedPaths []internal_io.MountedPath, useResponseFile bool) MessageTaskDispatch {
	return MessageTaskDispatch{
		Executable:      executable,
		Arguments:       arguments,
		WorkingDir:      workingDir,
		Environment:     env,
		MountedPaths:    mountedPaths,
		UseResponseFile: useResponseFile,

		timedMessageBody: newTimedMessageBody(),
	}
}
func (x *MessageTaskDispatch) Header() MessageType { return MSG_TASK_DISPATCH }
func (x *MessageTaskDispatch) Accept(wr MessageWriter) (err error) {
	if err = x.timedMessageBody.Accept(wr); err != nil {
		return
	}

	if !wr.ReadyForWork() {
		err = NewRemoteTaskError(wr, REMOTE_ERR_REFUSED, "not enough resources available")
		defer wr.Close(err)
		return WriteMessage(wr, NewMessageTaskStart(err))
	}

	if wr.Events().OnTaskDispatch == nil {
		err = NewRemoteTaskError(wr, REMOTE_ERR_INTERNAL, "no process dispatch available")
		defer wr.Close(err)
		return WriteMessage(wr, NewMessageTaskStart(err))
	}

	if err = WriteMessage(wr, NewMessageTaskStart(nil)); err != nil {
		return
	}

	var exitCode int32
	var processOpts internal_io.ProcessOptions
	processOpts.Init(
		internal_io.OptionProcessCaptureOutput,
		internal_io.OptionProcessExitCode(&exitCode),
		internal_io.OptionProcessEnvironment(x.Environment),
		internal_io.OptionProcessWorkingDir(x.WorkingDir),
		internal_io.OptionProcessMountedPath(x.MountedPaths...),
		internal_io.OptionProcessUseResponseFileIf(x.UseResponseFile),
		internal_io.OptionProcessFileAccess(func(far internal_io.FileAccessRecord) error {
			return WriteMessage(wr, NewMessageTaskFileAccess(far))
		}),
		internal_io.OptionProcessOutput(func(s string) error {
			return WriteMessage(wr, NewMessageTaskOutput(s))
		}))

	err = wr.Events().OnTaskDispatch(x.Executable, x.Arguments, &processOpts)

	if er := wr.Flush(); er != nil {
		return er
	}

	return WriteMessage(wr, NewMessageTaskStop(exitCode, err))
}
func (x *MessageTaskDispatch) Serialize(ar base.Archive) {
	x.timedMessageBody.Serialize(ar)
	ar.Serializable(&x.Executable)
	ar.Serializable(&x.Arguments)
	ar.Serializable(&x.Environment)
	base.SerializeSlice(ar, &x.MountedPaths)
	ar.Bool(&x.UseResponseFile)
	ar.Serializable(&x.WorkingDir)
}

type MessageTaskStart struct {
	errorMessageBody
}

func NewMessageTaskStart(err error) MessageTaskStart {
	return MessageTaskStart{newErrorMessageBody(err)}
}
func (x *MessageTaskStart) WasStarted() bool {
	switch x.ErrCode {
	case REMOTE_ERR_INTERNAL, REMOTE_ERR_PROCESS:
		return true
	case REMOTE_ERR_TIMEOUT, REMOTE_ERR_REFUSED:
		return false
	default:
		base.UnexpectedValuePanic(x.ErrCode, x.ErrCode)
		return true
	}
}
func (x *MessageTaskStart) Header() MessageType { return MSG_TASK_START }
func (x *MessageTaskStart) Accept(wr MessageWriter) (err error) {
	if err = x.errorMessageBody.Accept(wr); err == nil {
		err = wr.Events().OnTaskStart.Invoke(x)
	}
	return
}

type MessageTaskStop struct {
	ExitCode int32
	errorMessageBody
}

func NewMessageTaskStop(exitCode int32, err error) MessageTaskStop {
	return MessageTaskStop{
		ExitCode:         exitCode,
		errorMessageBody: newErrorMessageBody(err)}
}
func (x *MessageTaskStop) Header() MessageType { return MSG_TASK_STOP }
func (x *MessageTaskStop) Accept(wr MessageWriter) (err error) {
	defer wr.Close(err)
	err = x.errorMessageBody.Accept(wr)

	if er := wr.Events().OnTaskStop.Invoke(x); er != nil && err == nil {
		err = er
	}

	if er := wr.Flush(); er != nil {
		return er
	}

	return WriteMessage(wr, NewMessageGoodbye())
}
func (x *MessageTaskStop) Serialize(ar base.Archive) {
	x.errorMessageBody.Serialize(ar)
	ar.Int32(&x.ExitCode)
}

/***************************************
 * Batched Task Messages
 ***************************************/

type MessageTaskFileAccess struct {
	timedMessageBody
	Records []internal_io.FileAccessRecord
}

func NewMessageTaskFileAccess(far internal_io.FileAccessRecord) MessageTaskFileAccess {
	return MessageTaskFileAccess{
		Records:          []internal_io.FileAccessRecord{far},
		timedMessageBody: newTimedMessageBody(),
	}
}
func (x *MessageTaskFileAccess) Header() MessageType { return MSG_TASK_FILEACCESS }
func (x *MessageTaskFileAccess) Append(other MessageBody) bool {
	o := other.(*MessageTaskFileAccess)
	x.Timestamp = o.Timestamp
	x.Records = append(x.Records, o.Records...)
	total := 0
	for _, it := range x.Records {
		total += len(it.Path.Basename) + len(it.Path.Dirname.Path)
	}
	return total >= 4096 // flush after ~4096 characters
}
func (x *MessageTaskFileAccess) Accept(wr MessageWriter) (err error) {
	if err = x.timedMessageBody.Accept(wr); err == nil {
		err = wr.Events().OnTaskFileAccess.Invoke(x)
	}
	return
}
func (x *MessageTaskFileAccess) Serialize(ar base.Archive) {
	x.timedMessageBody.Serialize(ar)
	base.SerializeSlice(ar, &x.Records)
}

type MessageTaskOutput struct {
	Outputs []string
	timedMessageBody
}

func NewMessageTaskOutput(output string) (result MessageTaskOutput) {
	return MessageTaskOutput{
		Outputs:          []string{output},
		timedMessageBody: newTimedMessageBody(),
	}
}
func (x *MessageTaskOutput) Header() MessageType { return MSG_TASK_OUTPUT }
func (x *MessageTaskOutput) Append(other MessageBody) bool {
	o := other.(*MessageTaskOutput)
	x.Timestamp = o.Timestamp
	x.Outputs = append(x.Outputs, o.Outputs...)
	total := 0
	for _, it := range x.Outputs {
		total += len(it)
	}
	return total >= 4096 // flush after 4096 characters
}
func (x *MessageTaskOutput) Accept(wr MessageWriter) (err error) {
	if err = x.timedMessageBody.Accept(wr); err == nil {
		err = wr.Events().OnTaskOutput.Invoke(x)
	}
	return
}
func (x *MessageTaskOutput) Serialize(ar base.Archive) {
	x.timedMessageBody.Serialize(ar)
	base.SerializeMany(ar, ar.String, &x.Outputs)
}

/***************************************
 * Message Corpus
 ***************************************/

const enableMessageCorpusForZStd = false

var GetMessageCorpus = base.Memoize(func() *MessageCorpus {
	return &MessageCorpus{
		OutputDir: UFS.Output.Folder("MessageCorpus"),
	}
})

type MessageCorpus struct {
	OutputDir Directory
}

func (x *MessageCorpus) Add(body MessageBody) {
	if !enableMessageCorpusForZStd {
		return
	}

	randBytes := [16]byte{}
	rand.Read(randBytes[:])

	h := hex.EncodeToString(randBytes[:])
	f := x.OutputDir.Folder(h[0:2]).Folder(h[2:4]).File(h).ReplaceExt(".msg")

	UFS.CreateBuffered(f, func(w io.Writer) error {
		ar := base.NewArchiveBinaryWriter(w)

		header := body.Header()
		ar.Serializable(&header)
		ar.Serializable(body)

		return ar.Close()
	}, base.TransientPage4KiB)
}

/***************************************
 * Message Type
 ***************************************/

type MessageType int32

const (
	MSG_PING MessageType = iota
	MSG_PONG
	MSG_TASK_DISPATCH
	MSG_TASK_START
	MSG_TASK_FILEACCESS
	MSG_TASK_OUTPUT
	MSG_TASK_STOP
	MSG_GOODBYE
)

var MessageTypes = []MessageType{
	MSG_PING,
	MSG_PONG,
	MSG_TASK_DISPATCH,
	MSG_TASK_START,
	MSG_TASK_FILEACCESS,
	MSG_TASK_OUTPUT,
	MSG_TASK_STOP,
	MSG_GOODBYE,
}

func (x MessageType) Body(body func(MessageBody) error) error {
	switch x {
	case MSG_PING:
		return body(&MessagePing{})
	case MSG_PONG:
		return body(&MessagePong{})
	case MSG_TASK_DISPATCH:
		return body(&MessageTaskDispatch{})
	case MSG_TASK_START:
		return body(&MessageTaskStart{})
	case MSG_TASK_FILEACCESS:
		return body(&MessageTaskFileAccess{})
	case MSG_TASK_OUTPUT:
		return body(&MessageTaskOutput{})
	case MSG_TASK_STOP:
		return body(&MessageTaskStop{})
	case MSG_GOODBYE:
		return body(&MessageGoodbye{})
	default:
		return base.MakeUnexpectedValueError(x, x)
	}
}
func (x MessageType) String() string {
	switch x {
	case MSG_PING:
		return "PING"
	case MSG_PONG:
		return "PONG"
	case MSG_TASK_DISPATCH:
		return "TASK_DISPATCH"
	case MSG_TASK_START:
		return "TASK_START"
	case MSG_TASK_FILEACCESS:
		return "TASK_FILEACCESS"
	case MSG_TASK_OUTPUT:
		return "TASK_OUTPUT"
	case MSG_TASK_STOP:
		return "TASK_STOP"
	case MSG_GOODBYE:
		return "GOODBYE"
	default:
		base.UnexpectedValuePanic(x, x)
		return ""
	}
}
func (x *MessageType) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
