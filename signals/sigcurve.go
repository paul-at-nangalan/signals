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
	perfstats "github.com/paul-at-nangalan/stats/stats"
	"gonum.org/v1/gonum/stat"
	"io"
	"log"
	"math"
	"time"
)

const (
	LOGNONE = iota
	LOGINFO = iota
	LOGDBG  = iota
)

type SigCurve struct {
	variance            *managedslice.Slice
	variancetime        *managedslice.Slice
	variancecurve       *managedslice.Slice
	variancecurvedbg    *managedslice.Slice
	rsqrd               *managedslice.Slice
	sigbuyonvariance    bool
	sigsellonvariance   bool
	numorderbooksamples int
	//splitpoint          int
	minslope       float64
	window         int
	wndcounter     int
	averagewndsize int
	mindatapoints  int
	minrsqrd       float64 /// exclude very random data
	shiftfactor    float64

	statsvariancebuysig  *perfstats.Counter
	statsvariancesellsig *perfstats.Counter
	statsvaliddata       *perfstats.Counter
	statslopedata        *perfstats.BucketCounter
	statrsqrddata        *perfstats.BucketCounter
	loglevel             int

	datastore    store.Store
	storagename  string
	saveduration time.Duration
	lastsaved    time.Time
}

/*
*
numsamples - number of samples to use to keep
window - how many samples are used to calculat the slope
mindatapoints - the minimum number of data points before we do anything - 1 datapoint = 1 window of samples
minslope - what slope do you consider to be a upward/downward trend (and because it's time based, you'll need to experiment with real data)
minrsqrd - what is the min R squared value to accept as reliable (start with about 0.45)

To load/store from stored data use Restore - this is intended to be used after a restart so that it doesn't have to rebuild
all stats from nothing if there was a failure - it mainly is aimed at being able to do a simple restart on connection failure, where all the
subscriptions etc can make it difficult to do a reconnect
*/
func NewSigCurve(numsamples int, mindatapoints int, minslope float64, window int, minrsqrd float64) *SigCurve {
	return NewSigCurveWithFactor(numsamples, mindatapoints, minslope, window, minrsqrd, 1)
}

func NewSigCurveWithFactor(numsamples int, mindatapoints int, minslope float64, window int, minrsqrd float64,
	shiftfactor float64) *SigCurve {
	if mindatapoints >= numsamples {
		log.Panic("Splitpoint must be less than num data samples ", mindatapoints, ">=", numsamples)
	}
	if window >= numsamples-mindatapoints {
		log.Panic("The window should be much smaller than the difference between the number of samples and the mindatapoints")
	}
	if shiftfactor == 0 {
		shiftfactor = 1
	}

	sc := &SigCurve{
		variance:             managedslice.NewManagedSlice(0, numsamples),            ///we only need 2 * window here - but keep the rest for now for debug
		variancetime:         managedslice.NewManagedSlice(0, numsamples),            ///do LR against time on the x axis
		variancecurve:        managedslice.NewManagedSlice(0, (numsamples/window)+1), ///we need numsamples / window
		variancecurvedbg:     managedslice.NewManagedSlice(0, numsamples),            /// for printing
		rsqrd:                managedslice.NewManagedSlice(0, numsamples),            /// for printing
		shiftfactor:          shiftfactor,
		mindatapoints:        (mindatapoints / window) + 1, /// divde by the window to get it in block averages
		numorderbooksamples:  numsamples,
		minslope:             minslope,
		statsvariancebuysig:  perfstats.NewCounter("variance-buy-signalled"),
		statsvariancesellsig: perfstats.NewCounter("variance-sell-signalled"),
		window:               window,
		wndcounter:           0,
		averagewndsize:       ((numsamples - mindatapoints) / window) + 1,
		minrsqrd:             minrsqrd,
		statslopedata:        perfstats.NewBucketCounter(-100, 100, 4, "slope-stats"),
		statrsqrddata:        perfstats.NewBucketCounter(-1, 1, 0.05, "rsqrd-stats"),
		statsvaliddata:       perfstats.NewCounter("valid-data-sample"),
	}
	return sc
}

func (p *SigCurve) Encode(buffer io.Writer) {
	params := &bytes.Buffer{}
	enc := gob.NewEncoder(params)
	err := enc.Encode(p.numorderbooksamples)
	handlers.PanicOnError(err)
	err = enc.Encode(p.minslope)
	handlers.PanicOnError(err)
	err = enc.Encode(p.window)
	handlers.PanicOnError(err)
	err = enc.Encode(p.wndcounter)
	handlers.PanicOnError(err)
	err = enc.Encode(p.averagewndsize)
	handlers.PanicOnError(err)
	err = enc.Encode(p.mindatapoints)
	handlers.PanicOnError(err)
	err = enc.Encode(p.minrsqrd) /// exclude very random data
	handlers.PanicOnError(err)
	err = enc.Encode(p.shiftfactor)
	handlers.PanicOnError(err)
	err = enc.Encode(p.saveduration)
	handlers.PanicOnError(err)

	buffer.Write(params.Bytes())
}

func (p *SigCurve) Decode(buffer io.Reader) {
	enc := gob.NewDecoder(buffer)
	err := enc.Decode(&p.numorderbooksamples)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.minslope)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.window)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.wndcounter)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.averagewndsize)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.mindatapoints)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.minrsqrd) /// exclude very random data
	handlers.PanicOnError(err)
	err = enc.Decode(&p.shiftfactor)
	handlers.PanicOnError(err)
	err = enc.Decode(&p.saveduration)
	handlers.PanicOnError(err)
}

func (p *SigCurve) storeData() {
	if p.datastore == nil || p.lastsaved.Add(p.saveduration).After(time.Now()) {
		return
	}
	p.lastsaved = time.Now()
	p.datastore.Store(p.storagename+"-variance", p.variance)
	p.datastore.Store(p.storagename+"-variancetime", p.variancetime)
	p.datastore.Store(p.storagename+"-variancecurve", p.variancecurve)
	p.datastore.Store(p.storagename+"-variancecurvedbg", p.variancecurvedbg)
	p.datastore.Store(p.storagename+"-rsqrd", p.rsqrd)
	p.datastore.Store(p.storagename, p)
}

func (p *SigCurve) retrieveData(maxage time.Duration) (isvalid bool) {
	floatdecoder := storables.StorableFloat(0)
	timedecoder := storables.StorableTime{}
	p.variance, isvalid = managedslice.NewManagedSliceFromStore(p.storagename+"-variance", p.datastore, floatdecoder, maxage)
	if !isvalid {
		return false
	}
	p.variancetime, isvalid = managedslice.NewManagedSliceFromStore(p.storagename+"-variancetime", p.datastore, timedecoder, maxage)
	if !isvalid {
		return false
	}
	p.variancecurve, isvalid = managedslice.NewManagedSliceFromStore(p.storagename+"-variancecurve", p.datastore, floatdecoder, maxage)
	if !isvalid {
		return false
	}
	p.variancecurvedbg, isvalid = managedslice.NewManagedSliceFromStore(p.storagename+"-variancecurvedbg", p.datastore, floatdecoder, maxage)
	if !isvalid {
		return false
	}
	p.rsqrd, isvalid = managedslice.NewManagedSliceFromStore(p.storagename+"-rsqrd", p.datastore, floatdecoder, maxage)
	isvalid = p.datastore.Retrieve(p.storagename+"-rsqrd", maxage, p.rsqrd)
	if !isvalid {
		return false
	}
	isvalid = p.datastore.Retrieve(p.storagename, maxage, p)
	if !isvalid {
		return false
	}
	return true
}

// // This just sets up the storage - it won't save it
func (p *SigCurve) SetupStorage(storename string, fs store.Store, howoftentosave time.Duration) {
	p.storagename = storename
	p.datastore = fs
	p.saveduration = howoftentosave
	//// next time a stat comes in, we should save to storage
}

// /Optionally, try to load data from a store - make sure the name is unique
// / potentially slightly wasteful in terms of memory - but it should get cleaned up
func LoadFromStorage(storename string, fs store.Store, maxage time.Duration) (sigcurve *SigCurve, isvalid bool) {
	sigcurve = &SigCurve{ /// create an empty one and try to load data into it
		storagename:          storename,
		datastore:            fs,
		statsvariancebuysig:  perfstats.NewCounter("variance-buy-signalled"),
		statsvariancesellsig: perfstats.NewCounter("variance-sell-signalled"),
		wndcounter:           0,
		statslopedata:        perfstats.NewBucketCounter(-100, 100, 4, "slope-stats"),
		statrsqrddata:        perfstats.NewBucketCounter(-1, 1, 0.05, "rsqrd-stats"),
		statsvaliddata:       perfstats.NewCounter("valid-data-sample"),
	}
	isvalid = sigcurve.retrieveData(maxage)
	if !isvalid {
		return nil, false /// let it know the load failed - it maybe considered an error condition
	}
	return sigcurve, true
}

func (p *SigCurve) LogLevel(level int) {
	p.loglevel = level
}

func (p *SigCurve) logdbg(data ...interface{}) {
	if p.loglevel >= LOGDBG {
		fmt.Println(data...)
	}
}

func (p *SigCurve) GetStatsCounters() []perfstats.Stat {
	return []perfstats.Stat{p.statsvariancebuysig, p.statsvariancesellsig, p.statrsqrddata, p.statslopedata, p.statsvaliddata}
}

func (p *SigCurve) Plot() {
	///for testing/trialling different values
	fmt.Println("Raw data")
	dataplot.PlotManagedSlice(p.variance, 80, 40)
	fmt.Println()
	fmt.Println("Curve data")
	dataplot.PlotManagedSlice(p.variancecurvedbg, 80, 40)
	/*for _, f := range p.variancecurvedbg.Items() {
		fmt.Print(f, ",")
	}*/
	fmt.Println("Rsqrd data")
	dataplot.PlotManagedSlice(p.rsqrd, 80, 40)
}

func (p *SigCurve) linearRegressionFromArray(data []interface{}, sampletime []interface{}) (alpha, beta, rsqrd float64) {
	///data is our x
	// for now, the index of the data is y - so generate y
	firstsampletime := time.Time(sampletime[0].(storables.StorableTime))
	y := make([]float64, len(data))
	x := make([]float64, len(data))
	timerange := time.Time(sampletime[len(sampletime)-1].(storables.StorableTime)).Sub(firstsampletime)
	avgtime := float64(timerange) / float64(len(sampletime))
	for i, _ := range data {
		x[i] = float64(time.Time(sampletime[i].(storables.StorableTime)).Sub(firstsampletime)) / avgtime
		y[i] = float64(data[i].(storables.StorableFloat))
	}
	alpha, beta = stat.LinearRegression(x, y, nil, false)
	rsqrd = stat.RSquared(x, y, nil, alpha, beta)
	//p.logdbg("regression ", beta, rsqrd)
	p.statrsqrddata.Inc(rsqrd)
	return alpha, beta, rsqrd
}

func (p *SigCurve) trend() (isvalid, upwards bool) {
	/// provided we have more than
	if p.variancecurve.Len() >= p.mindatapoints {
		min := p.variancecurve.At(0).(storables.StorableFloat)
		max := p.variancecurve.At(0).(storables.StorableFloat)
		for _, f := range p.variancecurve.Items() {
			if f.(storables.StorableFloat) > max {
				max = f.(storables.StorableFloat)
			}
			if f.(storables.StorableFloat) < min {
				min = f.(storables.StorableFloat)
			}
		}
		angle := p.variancecurve.FromBack(0).(storables.StorableFloat)
		//// Scale the angle based on min and max angle
		scaled := angle / ((max - min) / 2)
		fmt.Println("Adding scaled slope stats ", scaled)
		p.statslopedata.Inc(float64(scaled))
		//see if the last item is a non-shallow upward curve
		if scaled > 0 {
			if float64(scaled) > p.minslope {
				return true, true
			}
		} else if scaled < 0 {
			if math.Abs(float64(scaled)) > p.minslope {
				return true, false
			}
		}
	} else {
		fmt.Println("Not enough data points ", p.variancecurve.Len(), p.mindatapoints)
	}
	return false, false
}

func (p *SigCurve) AddVarianceSample(variance float64, t time.Time) {
	//fmt.Println("Adding variance sample ", variance)
	p.storeData() /// this should only store data after a given duration
	p.variance.PushAndResize(storables.StorableFloat(variance * p.shiftfactor))
	p.variancetime.PushAndResize(storables.StorableTime(t))
	///Check the variance graph to see if we are on the way up
	p.wndcounter++
	if p.wndcounter > p.window {
		log.Panic("Wndcounter > window * 2: ", p.wndcounter, p.window)
	}
	///I need at least 2 windows to start
	// thereafter, create a record every new window
	if p.variance.Len() >= p.window && (p.wndcounter%p.window) == 0 {
		//p.logdbg("getting LR data")
		_, grad, rsqrd := p.linearRegressionFromArray(p.variance.Items()[p.variance.Len()-(p.window):],
			p.variancetime.Items()[p.variancetime.Len()-(p.window):])
		if !math.IsNaN(rsqrd) {
			p.rsqrd.PushAndResize(storables.StorableFloat(rsqrd))
		}
		/// don't push dubious results
		if rsqrd > p.minrsqrd {
			if math.IsNaN(grad) {
				log.Panicln("Grad is NaN ", grad)
			}
			//fmt.Println("adding LR data ", grad)
			p.variancecurve.PushAndResize(storables.StorableFloat(grad))
			p.variancecurvedbg.PushAndResize(storables.StorableFloat(grad))
		}
		p.wndcounter = 0
	} else {
		return
	}

	///Check we have enough samples - at least 2 * the split point
	if p.variancecurve.Len() < p.mindatapoints {
		p.logdbg("Data len less than min ", p.variancecurve.Len(), p.mindatapoints)
		return
	}
	sigbuyonvariance := false
	sigsellonvariance := false
	///need a better algo here

	///look for an inflection point
	// beta > 0 => upward slope
	// beta < 0 => downward slope
	// go backwards through the regression data to see if there's an inflection point
	isvalid, upwards := p.trend()

	if isvalid {
		p.statsvaliddata.Inc()
		///if the 2nd direction is upwards then signal buy else sell
		if upwards {
			/// we have a turning point
			//fmt.Println("Upturn curve detected ")
			sigbuyonvariance = true
			p.statsvariancebuysig.Inc()
		} else {
			//fmt.Println("Downturn curve detected ")
			/// we have a turning point
			sigsellonvariance = true
			p.statsvariancesellsig.Inc()
		}
	}
	p.sigbuyonvariance = sigbuyonvariance
	p.sigsellonvariance = sigsellonvariance
}

func (p *SigCurve) SigBuy() bool {
	return p.sigbuyonvariance
}
func (p *SigCurve) SigSell() bool {
	return p.sigsellonvariance
}
