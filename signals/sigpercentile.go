package signals

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/signals/dataplot"
	"github.com/paul-at-nangalan/signals/managedslice"
	"github.com/paul-at-nangalan/signals/signals/storables"
	"github.com/paul-at-nangalan/signals/store"
	"gonum.org/v1/gonum/stat"
	"io"
	"log"
	"math"
	"time"
)

const (
	FP_TOLERANCE = 0.0000000000001
)

type Bin struct {
	lowerval   float64
	upperval   float64
	count      float64
	lastupdate time.Time
}

func NewBin(start, interval, offest float64) *Bin {
	//fmt.Println("Create new bin at ", start, " with interval ", interval, " and offset ", offest)
	return &Bin{
		lowerval:   start + (interval * offest),
		upperval:   start + (interval * offest) + interval,
		count:      0,
		lastupdate: time.Now(),
	}
}

func (p *Bin) encode(enc *gob.Encoder) {
	err := enc.Encode(p.lowerval)
	handlers.PanicOnError(err)
	err = enc.Encode(p.upperval)
	handlers.PanicOnError(err)
	err = enc.Encode(p.count)
	handlers.PanicOnError(err)
	err = enc.Encode(p.lastupdate)
	handlers.PanicOnError(err)
}

func (p *Bin) decode(dec *gob.Decoder) {
	err := dec.Decode(&p.lowerval)
	handlers.PanicOnError(err)
	err = dec.Decode(&p.upperval)
	handlers.PanicOnError(err)
	err = dec.Decode(&p.count)
	handlers.PanicOnError(err)
	err = dec.Decode(&p.lastupdate)
	handlers.PanicOnError(err)
}

func (p *Bin) MidValue() float64 {
	return (p.lowerval + p.upperval) / 2
}

func (p *Bin) Add(val float64) {
	if val > p.upperval || val < p.lowerval {
		log.Panic("Adding val to bin outside range ", val, p)
	}
	p.lastupdate = time.Now()
	p.count++
}

func (p *Bin) TryAdd(val float64) bool {
	if val >= (p.lowerval-FP_TOLERANCE) && val <= (p.upperval+FP_TOLERANCE) {
		p.Add(val)
		return true
	}
	return false
}
func (p *Bin) Count() float64 {
	return p.count
}
func (p *Bin) LastUpdate() time.Duration {
	return time.Now().Sub(p.lastupdate)
}

type SigPercentile struct {
	buybelow  float64
	sellabove float64
	mindata   int

	upper, lower           float64
	issetupper, issetlower bool
	bins                   []*Bin
	targetnumbins          int
	pruneabove             int
	lastdata               *managedslice.Slice
	targetage              time.Duration

	sigbuy  bool
	sigsell bool

	datastore    store.Store
	storagename  string
	saveduration time.Duration
	lastsaved    time.Time
}

/*
*
targetage - the ideal age of data to calculate the percentile from - e.g. if you want to use ~1 days worth of data ideally, then set 1d
*/
func NewSigPercentile(buybelow, sellabove float64, mindata int, targetage time.Duration) *SigPercentile {
	if targetage == 0 {
		log.Panic("Target age is zero")
	}
	//// Don't create any bins until we have an idea of the range
	return &SigPercentile{
		buybelow:      buybelow,
		sellabove:     sellabove,
		bins:          make([]*Bin, 0),
		mindata:       mindata,
		targetnumbins: 1000,
		pruneabove:    2000,

		lastdata:  managedslice.NewManagedSlice(0, 2*mindata),
		targetage: targetage,
	}
}

// /Optionally, try to load data from a store - make sure the name is unique
// / potentially slightly wasteful in terms of memory - but it should get cleaned up
func LoadFromStorageSigPC(storename string, fs store.Store, maxage time.Duration) (sigpc *SigPercentile, isvalid bool) {
	sigpc = &SigPercentile{ /// create an empty one and try to load data into it
		storagename: storename,
		datastore:   fs,
	}
	isvalid = sigpc.retrieveData(maxage)
	if !isvalid {
		return nil, false /// let it know the load failed - it maybe considered an error condition
	}
	return sigpc, true
}

func (p *SigPercentile) Encode(buffer io.Writer) {
	params := &bytes.Buffer{}
	enc := gob.NewEncoder(params)
	err := enc.Encode(p.buybelow)
	handlers.PanicOnError(err)
	err = enc.Encode(p.sellabove)
	handlers.PanicOnError(err)
	err = enc.Encode(p.mindata)
	handlers.PanicOnError(err)
	err = enc.Encode(p.targetnumbins)
	handlers.PanicOnError(err)
	err = enc.Encode(p.pruneabove)
	handlers.PanicOnError(err)
	err = enc.Encode(p.targetage)
	handlers.PanicOnError(err)

	////Encode the bins
	lenbins := len(p.bins)
	err = enc.Encode(lenbins)
	handlers.PanicOnError(err)
	for _, bin := range p.bins {
		bin.encode(enc)
	}

	buffer.Write(params.Bytes())
}

func (p *SigPercentile) Decode(buffer io.Reader) {
	enc := gob.NewDecoder(buffer)
	err := enc.Decode(&p.buybelow)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.sellabove)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.mindata)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.targetnumbins)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.pruneabove)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.targetage)
	handlers.PanicOnError(err)

	lenbins := 0
	err = enc.Decode(&lenbins)
	handlers.PanicOnError(err)
	p.bins = make([]*Bin, lenbins)
	fmt.Println("Num bins ", lenbins)
	for i := range p.bins {
		p.bins[i] = &Bin{}
		p.bins[i].decode(enc)
	}
}

func (p *SigPercentile) storeData() {
	if p.datastore == nil || p.lastsaved.Add(p.saveduration).After(time.Now()) {
		return
	}
	p.datastore.Store(p.storagename+"-lastdata", p.lastdata)

	p.datastore.Store(p.storagename, p)
}

func (p *SigPercentile) retrieveData(maxage time.Duration) (isvalid bool) {
	floatdecoder := storables.StorableFloat(0)
	//timedecoder := StorableTime{}
	p.lastdata, isvalid = managedslice.NewManagedSliceFromStore(p.storagename+"-lastdata", p.datastore, floatdecoder, maxage)
	if !isvalid {
		return false
	}
	p.datastore.Retrieve(p.storagename, maxage, p)
	return true
}

// // This just sets up the storage - it won't save it
func (p *SigPercentile) SetupStorage(storename string, fs store.Store, howoftentosave time.Duration) {
	p.storagename = storename
	p.datastore = fs
	p.saveduration = howoftentosave
	//// next time a stat comes in, we should save to storage
}

func (p *SigPercentile) Plot() {
	bins := make([]float64, len(p.bins))
	for i, bin := range p.bins {
		bins[i] = bin.Count()
	}
	dataplot.Plot(bins, 40, 40)
}

func (p *SigPercentile) SetRange(val float64) {
	if !p.issetupper || val > p.upper {
		p.upper = val
		p.issetupper = true
	}
	if !p.issetlower || val < p.lower {
		p.lower = val
		p.issetlower = true
	}
}

func (p *SigPercentile) predictIndex(val float64) (predicted int, outofbounds float64) {
	if val > p.upper {
		return 0, val - p.upper
	}
	if val < p.lower {
		return 0, val - p.lower
	}
	r := p.upper - p.lower
	interval := r / float64(len(p.bins))
	offset := (val - p.lower) / interval
	if offset > 0 {
		offset-- /// just in case we end up right on the upper edge
	}
	if math.IsNaN(offset) {
		log.Panic("Predicted index gives neg offset ", val, p.lower, p.upper, len(p.bins))
	}
	return int(offset), 0 /// return the lower expected index - then we can just search up
}

func (p *SigPercentile) addBucket(val float64) {
	if val > p.lower && val < p.upper {
		log.Panic("val within range")
	}
	r := p.upper - p.lower
	interval := r / float64(len(p.bins))
	diff := p.lower - val /// assume val < lower
	if val > p.upper {
		diff = val - p.upper
	}
	extrabins := int(diff/interval) + 1
	newbins := make([]*Bin, extrabins)
	currlen := len(p.bins)
	p.bins = append(p.bins, newbins...)
	if val < p.lower {
		///shift data up and prepend
		copy(p.bins[extrabins:], p.bins[:currlen])
		start := p.lower - (interval * float64(extrabins))
		for i := 0; i < extrabins; i++ {
			bin := NewBin(start, interval, float64(i))
			p.bins[i] = bin
		}
		p.lower = start
	} else {
		end := p.upper + (interval * float64(extrabins))
		for i := 0; i < extrabins; i++ {
			bin := NewBin(p.upper, interval, float64(i))
			p.bins[currlen+i] = bin
		}
		p.upper = end
	}

}

func (p *SigPercentile) tryAddFromIndx(val float64, predictedindex int) bool {
	if math.IsNaN(val) {
		log.Panic("NaN fed into try Add From Indx")
	}
	if predictedindex < 0 {
		log.Panic("Predicted index is screwed up ", predictedindex)
	}
	counter := 0
	for i := predictedindex; i < len(p.bins); i++ {
		if p.bins[i].TryAdd(val) {
			/// we're done - return
			return true
		}
		counter++
		if counter > 5 {
			log.Panic("Somethings wrong with the index prediction ", val, predictedindex, p.lower, p.upper,
				p.bins[predictedindex])
		}
	}
	return false
}

func (p *SigPercentile) prune() {
	///see if we can prune from the end of the array
	countupper := 0
	countlower := 0
	for i := 0; i < len(p.bins); i++ {
		t := p.bins[len(p.bins)-(i+1)].LastUpdate()
		if t > p.targetage {
			countupper++
		} else {
			break
		}

	}
	for i := 0; i < len(p.bins); i++ {
		t := p.bins[i].LastUpdate()
		if t > p.targetage {
			countlower++
		} else {
			break
		}
	}
	if countupper > 0 {
		p.bins = p.bins[:len(p.bins)-countupper]
		p.upper = p.bins[len(p.bins)-1].upperval
	}
	if countlower > 0 {
		///Do a memory swap to avoid loosing memory of the bottom (effectively a memory leak)
		newbins := make([]*Bin, len(p.bins)-countlower)
		copy(newbins, p.bins[countlower:])
		p.bins = newbins
		p.lower = p.bins[0].lowerval
	}
}

func (p *SigPercentile) AddData(val float64) {
	p.storeData()
	if math.IsNaN(val) {
		/// we can't handle this - so drop it and hope its the only one
		log.Println("WARNING NaN passed to SigPercentile: AddData")
		return
	}
	p.lastdata.PushAndResize(storables.StorableFloat(val))
	if p.lastdata.Len() < p.mindata {
		p.SetRange(val)
		return
	}
	/// if we have nothing - create
	if len(p.bins) == 0 {
		///set range one more time in case this last dp is an outlier
		p.SetRange(val)
		fmt.Println("Creating bins")
		interval := (p.upper - p.lower) / float64(p.targetnumbins)
		p.bins = make([]*Bin, p.targetnumbins)
		for i, _ := range p.bins {
			p.bins[i] = NewBin(p.lower, interval, float64(i))
		}
		for _, val := range p.lastdata.Items() {
			predictedindex, outofbounds := p.predictIndex(float64(val.(storables.StorableFloat)))
			if outofbounds != 0 {
				log.Panic("oob is still non zero, ", val, outofbounds, p.lower, p.upper, len(p.bins))
			}
			if !p.tryAddFromIndx(float64(val.(storables.StorableFloat)), predictedindex) {
				log.Panic("failed to add value from last data ", val, predictedindex, p.lower, p.upper)
			}
		}
		fmt.Println("Done ", p.lower, p.upper)
		return
	}
	///// Add the value - extending bins if needed
	predictedindex, outofbounds := p.predictIndex(val)
	if outofbounds != 0 {
		//fmt.Println("Add new bucket for ", val)
		/// add buckets and prune
		p.addBucket(val)
		predictedindex, outofbounds = p.predictIndex(val)
		if outofbounds != 0 {
			log.Panic("oob is still non zero, ", val, outofbounds)
		}
	}
	if val < p.lower || val > p.upper {
		log.Panic("trying to add a value that's outside range ", val, p.lower, p.upper)
	}

	if !p.tryAddFromIndx(val, predictedindex) {
		log.Panic("Failed to place val after adding bins ", p.lower, p.upper, val)
	}
	if len(p.bins) > p.pruneabove {
		p.prune()
	}
	p.checkData(val)
}

func (p *SigPercentile) checkData(val float64) float64 {
	vals := make([]float64, len(p.bins))
	weights := make([]float64, len(p.bins))
	for i, bin := range p.bins {
		vals[i] = bin.MidValue()
		weights[i] = bin.Count()
	}
	place := stat.CDF(val, stat.Empirical, vals, weights)
	sigsell := false
	sigbuy := false
	if place > p.sellabove {
		sigsell = true
	}
	if place < p.buybelow {
		sigbuy = true
	}
	p.sigbuy = sigbuy
	p.sigsell = sigsell
	return place
}

func (p *SigPercentile) SigBuy() bool {
	return p.sigbuy
}
func (p *SigPercentile) SigSell() bool {
	return p.sigsell
}
