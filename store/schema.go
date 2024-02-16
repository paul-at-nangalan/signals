package store

import (
	"io"
	"time"
)

type Store interface {
	Store(name string, data Encoder)
	Retrieve(name string, maxage time.Duration, t Decoder) (isvalid bool)
}

type Decoder interface {
	Decode(buffer io.Reader)
}

type Encoder interface {
	Encode(buffer io.Writer)
}
