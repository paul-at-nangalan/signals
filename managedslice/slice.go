package managedslice

import (
	"encoding/gob"
	"fmt"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/signals/store"
	"io"
	"log"
	"time"
)

type ItemCoder interface {
	Decode(buffer *gob.Decoder) any
	Encode(buffer *gob.Encoder)
}

type Slice struct {
	slice      []interface{}
	origslice  []interface{}
	maxsize    int
	maxactual  int
	actualsize int

	decoder ItemCoder
}

func (p *Slice) Decode(buffer io.Reader) {
	dec := gob.NewDecoder(buffer)
	err := dec.Decode(&p.maxsize)
	handlers.PanicOnError(err)
	err = dec.Decode(&p.maxactual)
	handlers.PanicOnError(err)
	err = dec.Decode(&p.actualsize)
	handlers.PanicOnError(err)
	slicelen := int(0)
	err = dec.Decode(&slicelen)
	handlers.PanicOnError(err)
	p.slice = make([]interface{}, slicelen)
	fmt.Println("Decode slice len ", slicelen)
	for i := 0; i < slicelen; i++ {
		item := p.decoder.Decode(dec)
		//fmt.Println("Decoded ", item)
		_, ok := item.(ItemCoder)
		if !ok {
			log.Panic("Item returned by ItemCoder does not implement the store.Encoder interface. This cannot be the same item that was stored ",
				item)
		}
		p.slice[i] = item
	}
}

func (p *Slice) Encode(buffer io.Writer) {
	enc := gob.NewEncoder(buffer)
	err := enc.Encode(p.maxsize)
	handlers.PanicOnError(err)
	err = enc.Encode(p.maxactual)
	handlers.PanicOnError(err)
	err = enc.Encode(p.actualsize)
	handlers.PanicOnError(err)
	err = enc.Encode(len(p.slice))
	handlers.PanicOnError(err)
	//fmt.Println("Encode slice len ", len(p.slice))
	for _, val := range p.slice {
		val.(ItemCoder).Encode(enc) //// val must be encodable - if not a standard type, then support the gob interface
	}
}

const (
	MULTIPLIER = 3
)

func NewManagedSlice(size int, maxsize int) *Slice {
	ms := &Slice{}
	ms.slice = make([]interface{}, size, maxsize*MULTIPLIER)
	ms.origslice = ms.slice[:0] ///this should always point at the original slice
	ms.maxsize = maxsize
	ms.actualsize = size
	ms.maxactual = maxsize * MULTIPLIER
	return ms
}

func NewManagedSliceFromStore(storename string, fs store.Store, itemdecoder ItemCoder, maxage time.Duration) (ms *Slice, isvalid bool) {
	ms = &Slice{
		decoder: itemdecoder,
	}
	isvalid = fs.Retrieve(storename, maxage, ms)
	return ms, isvalid
}

func (p *Slice) Store(storename string, fs store.Store) {
	fs.Store(storename, p)
}

func (p *Slice) Set(index int, item interface{}) {
	p.slice[index] = item
}
func (p *Slice) At(index int) interface{} {
	return p.slice[index]
}
func (p *Slice) FromBack(index int) interface{} {
	return p.slice[len(p.slice)-(index+1)]
}
func (p *Slice) Len() int {
	return len(p.slice)
}
func (p *Slice) Items() []interface{} {
	return p.slice
}

func (p *Slice) PushAndResize(item interface{}) (first interface{}) {
	p.slice = append(p.slice, item)
	p.actualsize++
	if len(p.slice) > p.maxsize {
		first = p.slice[0]
		p.slice = p.slice[1:]
	}
	if p.actualsize > p.maxactual {
		///reallocate
		oldslice := p.slice
		newslice := p.origslice
		newslice = append(newslice, oldslice...)
		if len(p.slice) != len(newslice) {
			log.Panic("Slice reallocate size mismatch ", len(p.slice), len(newslice))
		}
		p.slice = newslice
		p.actualsize = len(newslice)
	}
	return first
}

// / Warning - SLOW
func (p *Slice) Rem(item interface{}) {
	strtlen := len(p.slice)
	for i, elem := range p.slice {
		if item == elem {
			copy(p.slice[i:], p.slice[i+1:])
			p.actualsize--
			p.slice = p.slice[:strtlen-1]
			return
		}
	}
	log.Panicln("Trying to remove element that's not in the array ", item)
}
