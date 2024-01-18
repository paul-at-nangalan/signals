package managedslice

import (
	"log"
)

type Slice struct {
	slice      []interface{}
	origslice  []interface{}
	maxsize    int
	maxactual  int
	actualsize int
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
