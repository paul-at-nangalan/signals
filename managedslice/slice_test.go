package managedslice

import (
	"encoding/gob"
	"fmt"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/short-term-store/store"
	"gotest.tools/v3/assert"
	"runtime"
	"testing"
	"time"
)

func test(s *Slice, expect []int, t *testing.T) {
	for i := 0; i < s.Len(); i++ {
		if s.At(i) != expect[i] {
			t.Error("Mismatch at ", i, s.At(i), " != ", expect[i])
		}
	}
}

func TestSlice_At(t *testing.T) {
	s := NewManagedSlice(0, 3)
	for i := 0; i < 25; i++ {
		s.PushAndResize(i + 1)
	}
	expect := []int{23, 24, 25}
	test(s, expect, t)
}

func TestSlice_Rem(t *testing.T) {
	s := NewManagedSlice(0, 6)
	for i := 0; i < 12; i++ {
		s.PushAndResize(i + 1)
	}
	s.Rem(10)
	expect := []int{7, 8, 9, 11, 12}
	test(s, expect, t)
	fmt.Println("Test add after rem")
	for i := 12; i < 14; i++ {
		s.PushAndResize(i + 1)
	}
	expect = []int{8, 9, 11, 12, 13, 14}
	test(s, expect, t)
	s.Rem(14)
	expect = []int{8, 9, 11, 12, 13}
	test(s, expect, t)
}

func TestSlice_PushAndResize(t *testing.T) {
	s := NewManagedSlice(0, 10)
	for i := 0; i < 2000; i++ {
		s.PushAndResize(i + 1)
	}
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	expect := []int{1991, 1992, 1993, 1994, 1995, 1996, 1997, 1998, 1999, 2000}
	test(s, expect, t)
	if len(s.slice) > 10 {
		t.Error("Failing to de-alloc memory", len(s.slice))
	}

	for i := 0; i < 200000; i++ {
		s.PushAndResize(i + 1)
	}
	for i := 0; i < 200000; i++ {
		s.PushAndResize(i + 1)
	}
	if cap(s.origslice) > 30 {
		t.Error("Heap usage seems to have changed ", cap(s.origslice))
	}
	expect = []int{199991, 199992, 199993, 199994, 199995, 199996, 199997, 199998, 199999, 200000}
	test(s, expect, t)
}

func Test_FromBack(t *testing.T) {
	s := NewManagedSlice(0, 5)
	data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for i, val := range data {
		if i == 5 {
			break
		}
		s.PushAndResize(val)
	}
	back := s.FromBack(0)
	if back.(int) != 5 {
		t.Error("Mismatch value exp 5 ", back)
	}
	back = s.FromBack(1)
	if back.(int) != 4 {
		t.Error("Mismatch value exp 4 ", back)
	}
	for _, val := range data {
		s.PushAndResize(val)
	}
	back = s.FromBack(0)
	if back.(int) != 10 {
		t.Error("Mismatch value exp 10 ", back)
	}
	back = s.FromBack(1)
	if back.(int) != 9 {
		t.Error("Mismatch value exp 9 ", back)
	}

}

type TestEncDec struct {
	val float64
	x   float64
}

func (t *TestEncDec) Encode(enc *gob.Encoder) {
	err := enc.Encode(t.val)
	handlers.PanicOnError(err)
	err = enc.Encode(t.x)
	handlers.PanicOnError(err)
}

func (*TestEncDec) Decode(dec *gob.Decoder) any {
	t := &TestEncDec{}
	err := dec.Decode(&t.val)
	handlers.PanicOnError(err)
	err = dec.Decode(&t.x)
	handlers.PanicOnError(err)
	return t
}

func Test_SliceEncodeDecode(t *testing.T) {
	ms := NewManagedSlice(0, 20)
	testdata := make([]TestEncDec, 100)
	for i := range testdata {
		testdata[i].x = float64(i) * 0.1
		testdata[i].val = float64(i) * 0.2
		ms.PushAndResize(&testdata[i])
	}
	fs := store.NewFileStore("/tmp/teststore/")
	ms.Store("test-managed-slice", fs)
	time.Sleep(5 * time.Second)

	testdec := &TestEncDec{}
	restored, isvalid := NewManagedSliceFromStore("test-managed-slice", fs, testdec, time.Hour)
	assert.Equal(t, isvalid, true, "Data is marked invalid")
	////restored should contain the last 20 entries
	assert.Equal(t, restored.Len(), 20, "Mismatch length")
	expdata := testdata[80:]
	for i, data := range expdata {
		retrieved := restored.At(i)
		assert.Equal(t, retrieved.(*TestEncDec).x, data.x, "Mismatch on data at ", i)
		assert.Equal(t, retrieved.(*TestEncDec).val, data.val, "Mismatch on data at ", i)
	}
}
