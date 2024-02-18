package storables

import (
	"encoding/gob"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"time"
)

type StorableFloat float64

func (StorableFloat) Decode(buffer *gob.Decoder) any {
	var f float64
	err := buffer.Decode(&f)
	handlers.PanicOnError(err)
	return StorableFloat(f)
}

func (f StorableFloat) Encode(buffer *gob.Encoder) {
	err := buffer.Encode(float64(f))
	handlers.PanicOnError(err)
}

type StorableTime time.Time

func (StorableTime) Decode(buffer *gob.Decoder) any {
	var t time.Time
	err := buffer.Decode(&t)
	handlers.PanicOnError(err)
	return StorableTime(t)
}

func (f StorableTime) Encode(buffer *gob.Encoder) {
	err := buffer.Encode(time.Time(f))
	handlers.PanicOnError(err)
}
