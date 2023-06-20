package cluster

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/quic-go/quic-go"

	//lint:ignore ST1001 ignore dot imports warning

	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Tunnel
 ***************************************/

type TunnelEvents struct {
	OnError          PublicEvent[error]
	OnTaskStart      PublicEvent[*MessageTaskStart]
	OnTaskStop       PublicEvent[*MessageTaskStop]
	OnTaskFileAccess PublicEvent[*MessageTaskFileAccess]
	OnTaskOutput     PublicEvent[*MessageTaskOutput]

	OnReadyForWork func() bool
	OnTaskDispatch RunProcessFunc
}

type Tunnel struct {
	Cluster     *Cluster
	Compression CompressionOptions

	conn   quic.Connection
	stream quic.Stream

	ping    time.Duration
	timeout time.Duration

	lastRead  time.Time
	lastWrite time.Time

	TunnelEvents
}

func NewDialTunnel(cluster *Cluster, ctx context.Context, addr string, timeout time.Duration, compression ...CompressionOptionFunc) (tunnel *Tunnel, err error) {
	LogVeryVerbose(LogCluster, "dialing remote peer %q", addr)

	dialer, err := quic.DialAddr(ctx, addr, generateClientTLSConfig(), nil)
	if err != nil {
		return nil, err
	}

	stream, err := dialer.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	return newTunnel(cluster, dialer, stream, timeout, compression...), nil
}
func NewListenTunnel(cluster *Cluster, ctx context.Context, conn quic.Connection, timeout time.Duration, compression ...CompressionOptionFunc) (*Tunnel, error) {
	LogVeryVerbose(LogCluster, "accept remote peer %q", conn.RemoteAddr())

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	return newTunnel(cluster, conn, stream, timeout, compression...), nil
}

func newTunnel(cluster *Cluster, conn quic.Connection, stream quic.Stream, timeout time.Duration, compression ...CompressionOptionFunc) *Tunnel {
	utc := time.Now().UTC()
	return &Tunnel{
		Cluster:     cluster,
		Compression: NewCompressionOptions(compression...),
		conn:        conn,
		stream:      stream,
		lastRead:    utc,
		lastWrite:   utc,
		ping:        timeout,
		timeout:     timeout,
	}
}
func (x *Tunnel) TimeSinceLastRead() time.Duration {
	return time.Now().UTC().Sub(x.lastRead)
}
func (x *Tunnel) TimeSinceLastWrite() time.Duration {
	return time.Now().UTC().Sub(x.lastWrite)
}
func (x *Tunnel) Read(buf []byte) (int, error) {
	if x.stream == nil {
		return 0, io.ErrUnexpectedEOF
	}

	if err := x.stream.SetReadDeadline(time.Now().Add(x.timeout)); err != nil {
		return 0, err
	}

	n, err := x.Cluster.StreamRead(x.stream, buf)
	if err == nil {
		x.lastRead = time.Now().UTC()
	}
	return n, err
}
func (x *Tunnel) Write(buf []byte) (int, error) {
	if x.stream == nil {
		return 0, io.ErrUnexpectedEOF
	}

	if err := x.stream.SetWriteDeadline(time.Now().Add(x.timeout)); err != nil {
		return 0, err
	}

	n, err := x.Cluster.StreamWrite(x.stream, buf)
	if err == nil {
		x.lastWrite = time.Now().UTC()
		if n < len(buf) {
			err = io.ErrShortWrite
		}
	}
	return n, err
}
func (x *Tunnel) Close() error {
	var streamErr, connErr error

	if x.stream != nil {
		streamErr = x.stream.Close()
		x.stream = nil
	}
	if x.conn != nil {
		connErr = x.conn.CloseWithError(0, "close(pipe)")
		x.conn = nil
	}

	if streamErr == nil {
		return connErr
	} else {
		return streamErr
	}
}

/***************************************
 * TLS Config
 ***************************************/

const TUNNEL_QUIC_PROTOCOL = "quic-ppb-task-distribution"

var getTunnelQuicProtocol = Memoize[string](func() string {
	return fmt.Sprint(TUNNEL_QUIC_PROTOCOL, `-`, CurrentHost().String())
})

func generateClientTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{getTunnelQuicProtocol()},
	}
}

// Setup a bare-bones TLS config for the server
func generateServerTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		CommandPanic(err)
	}

	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		CommandPanic(err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		CommandPanic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{getTunnelQuicProtocol()},
	}
}
