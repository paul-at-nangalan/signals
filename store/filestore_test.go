package store

import (
	"encoding/gob"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"gotest.tools/v3/assert"
	"io"
	"testing"
	"time"
)

type DataForTest struct {
	id  int
	str string
	zzz []float64
}

func (p *DataForTest) Decode(buffer io.Reader) {
	dec := gob.NewDecoder(buffer)
	err := dec.Decode(&p.id)
	handlers.PanicOnError(err)
	err = dec.Decode(&p.str)
	handlers.PanicOnError(err)
	arrlen := 0
	err = dec.Decode(&arrlen)
	zzz := make([]float64, arrlen)
	for i := range zzz {
		err = dec.Decode(&zzz[i])
	}
	p.zzz = zzz
}

func (p *DataForTest) Encode(buffer io.Writer) {
	//TODO implement me
	enc := gob.NewEncoder(buffer)
	err := enc.Encode(p.id)
	handlers.PanicOnError(err)
	err = enc.Encode(p.str)
	handlers.PanicOnError(err)
	err = enc.Encode(len(p.zzz))
	for i := range p.zzz {
		err = enc.Encode(p.zzz[i])
		handlers.PanicOnError(err)
	}
}

func isEqual(exp, got *DataForTest, t *testing.T) {
	assert.Equal(t, exp.id, got.id, "Mismatch on ID")
	assert.Equal(t, exp.str, got.str, "Mismatch on str")
	for i, z := range exp.zzz {
		assert.Equal(t, z, got.zzz[i], "Mismatch on zzz")
	}
	assert.Equal(t, len(exp.zzz), len(got.zzz), "Mismatch on len zzz")
}

func Test_StoreAndRetrieve(t *testing.T) {
	data := &DataForTest{
		id:  23,
		str: "this is a string",
		zzz: []float64{1.2, 1.5, 33.009},
	}
	data2 := &DataForTest{
		id:  102,
		str: "something else",
		zzz: []float64{99.9, 21.25, 233.009},
	}
	data3 := &DataForTest{
		id:  11202,
		str: "something else again",
		zzz: []float64{99.9, 99, 233.009},
	}
	fs := NewFileStore("/tmp/filestore/test/")
	fs.Store("data1", data)
	fs.Store("data3", data3)
	time.Sleep(time.Second) /// give it chance to save the data

	retrieved := DataForTest{}
	isvalid := fs.Retrieve("data1", time.Hour, &retrieved)
	assert.Equal(t, isvalid, true, "Retrieve returned !isvalid")
	isEqual(data, &retrieved, t)

	fs.Store("data1", data2)
	time.Sleep(time.Second) /// give it chance to save the data

	isvalid = fs.Retrieve("data1", time.Hour, &retrieved)
	assert.Equal(t, isvalid, true, "Retrieve returned !isvalid")
	isEqual(data2, &retrieved, t)

	isvalid = fs.Retrieve("data3", time.Hour, &retrieved)
	assert.Equal(t, isvalid, true, "Retrieve returned !isvalid")
	isEqual(data3, &retrieved, t)

}

func Test_Timeout(t *testing.T) {
	data := &DataForTest{
		id:  23,
		str: "this is a string",
		zzz: []float64{1.2, 1.5, 33.009},
	}
	fs := NewFileStore("/tmp/filestore/test/")
	fs.Store("blue", data)
	time.Sleep(3 * time.Second)
	retrieve := DataForTest{}
	isvalid := fs.Retrieve("blue", time.Second, &retrieve)
	assert.Equal(t, isvalid, false, "Is valid should be false")
}
