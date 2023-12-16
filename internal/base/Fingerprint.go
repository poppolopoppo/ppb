package base

import (
	"encoding/hex"
	"fmt"
	"hash"
	"io"

	"github.com/minio/sha256-simd"
)

var LogFingerprint = NewLogCategory("Fingerprint")

/***************************************
 * Fingerprint
 ***************************************/

type Fingerprint [sha256.Size]byte

func (x *Fingerprint) Serialize(ar Archive) {
	ar.Raw(x[:])
}
func (x Fingerprint) Slice() []byte {
	return x[:]
}
func (x Fingerprint) Guid() string {
	return fmt.Sprint("{",
		hex.EncodeToString(x[0:4]),
		"-",
		hex.EncodeToString(x[4:6]),
		"-",
		hex.EncodeToString(x[6:8]),
		"-",
		hex.EncodeToString(x[8:10]),
		"-",
		hex.EncodeToString(x[10:16]),
		"}")
}
func (x Fingerprint) String() string {
	return hex.EncodeToString(x[:])
}
func (x Fingerprint) ShortString() string {
	return hex.EncodeToString(x[:8])
}
func (x Fingerprint) Valid() bool {
	for _, it := range x {
		if it != 0 {
			return true
		}
	}
	return false
}
func (d *Fingerprint) Set(str string) (err error) {
	var data []byte
	if data, err = hex.DecodeString(str); err == nil {
		if len(data) == sha256.Size {
			copy(d[:], data)
			return nil
		} else {
			err = fmt.Errorf("fingerprint: unexpected string length '%s'", str)
		}
	}
	return err
}
func (x Fingerprint) MarshalText() ([]byte, error) {
	buf := [sha256.Size * 2]byte{}
	Assert(func() bool { return hex.EncodedLen(len(x[:])) == len(buf) })
	hex.Encode(buf[:], x[:])
	return buf[:], nil
}
func (x *Fingerprint) UnmarshalText(data []byte) (err error) {
	n, err := hex.Decode(x.Slice(), data)
	if err == nil && n != sha256.Size {
		err = fmt.Errorf("fingerprint: unexpected string length '%s'", data)
	}
	return err
}

/***************************************
 * Serializable Fingerprint
 ***************************************/

var DigesterPool = NewRecycler(
	func() hash.Hash {
		return sha256.New()
	},
	func(digester hash.Hash) {
		digester.Reset()
	})

func SerializeAnyFingerprint(any func(ar Archive) error, seed Fingerprint) (result Fingerprint, err error) {
	digester := DigesterPool.Allocate()
	defer DigesterPool.Release(digester)

	if _, err = digester.Write(seed[:]); err != nil {
		return
	}

	ar := NewArchiveBinaryWriter(digester, AR_DETERMINISM)
	defer ar.Close()

	if err = any(&ar); err != nil {
		return
	}
	LogPanicIfFailed(LogFingerprint, ar.Error())

	copy(result[:], digester.Sum(nil))
	return
}

func ReaderFingerprint(rd io.Reader, seed Fingerprint) (result Fingerprint, err error) {
	digester := DigesterPool.Allocate()
	defer DigesterPool.Release(digester)

	digester.Write(seed[:])

	if _, err = TransientIoCopy(digester, rd); err == nil {
		copy(result[:], digester.Sum(nil))
		Assert(result.Valid)
	}

	return
}

func StringFingerprint(in string) Fingerprint {
	tmp := TransientBuffer.Allocate()
	defer TransientBuffer.Release(tmp)
	tmp.WriteString(in) // avoid allocating a new slice to convert string to []byte
	return sha256.Sum256(tmp.Bytes())
}

func SerializeFingerpint(value Serializable, seed Fingerprint) Fingerprint {
	fingerprint, err := SerializeAnyFingerprint(func(ar Archive) error {
		ar.Serializable(value)
		return nil
	}, seed)
	LogPanicIfFailed(LogFingerprint, err)
	return fingerprint
}
