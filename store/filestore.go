package store

import (
	"bytes"
	"fmt"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Blob struct {
	name string
	buff *bytes.Buffer
}

// // DO NOT READ AND WRITE AT THE SAME TIME
type FileStore struct {
	filename  string
	file      *os.File
	queue     chan Blob
	quit      chan bool
	ready     chan bool
	isrunning bool
}

func NewFileStore(filename string) *FileStore {
	err := os.MkdirAll(filename, 0750)
	handlers.PanicOnError(err)
	fs := &FileStore{
		filename: filename,
		queue:    make(chan Blob, 20),
		quit:     make(chan bool),
		ready:    make(chan bool),
	}
	go fs.run()
	<-fs.ready ///make sure we're started before returning
	return fs
}

func (p *FileStore) Close() {
	p.quit <- true
}

func (p *FileStore) run() {
	defer handlers.HandlePanic()
	defer func() {
		p.isrunning = false
	}()
	p.ready <- false
	p.isrunning = true
	for {
		select {
		case blob := <-p.queue:
			path := filepath.Join(p.filename, blob.name)
			f, err := os.Create(path)
			handlers.PanicOnError(err)
			fmt.Println("Writing out ", len(blob.buff.Bytes()), " for ", blob.name)
			_, err = f.Write(blob.buff.Bytes())
			handlers.PanicOnError(err)
			f.Close()
		case <-p.quit:
			return
		}
	}
}

func (p *FileStore) Store(name string, data Encoder) {
	if !p.isrunning {
		log.Panicln("File store is not active")
	}
	buffer := &bytes.Buffer{}
	data.Encode(buffer)
	p.queue <- Blob{
		name: name,
		buff: buffer,
	}
}

func (p *FileStore) Retrieve(name string, maxage time.Duration, t Decoder) (isvalid bool) {
	path := filepath.Join(p.filename, name)
	file, err := os.Stat(path)
	if err != nil {
		log.Println("No pre-stored file for ", name, ", returning nil")
		return false
	}
	if file.ModTime().Add(maxage).Before(time.Now()) {
		log.Println("Prestored file is too old ", file.ModTime(), time.Now())
		return false
	}
	fmt.Println("Reading store from file of size ", file.Size(), " for ", name)
	f, err := os.Open(path)
	handlers.PanicOnError(err)
	defer f.Close()
	t.Decode(f)
	return true
}
