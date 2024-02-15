package signals

import (
	"fmt"
	"github.com/paul-at-nangalan/signals/dataplot"
	"github.com/paul-at-nangalan/signals/managedslice"
	perfstats "github.com/paul-at-nangalan/stats/stats"
	"gonum.org/v1/gonum/stat"
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

	statsvariancebuysig  *perfstats.Counter
	statsvariancesellsig *perfstats.Counter
	statsvaliddata       *perfstats.Counter
	statslopedata        *perfstats.BucketCounter
	statrsqrddata        *perfstats.BucketCounter
	loglevel             int
}

/*
*
numsamples - number of samples to use to keep
window - how many samples are used to calculat the slope
mindatapoints - the minimum number of data points before we do anything - 1 datapoint = 1 window of samples
minslope - what slope do you consider to be a upward/downward trend (and because it's time based, you'll need to experiment with real data)
minrsqrd - what is the min R squared value to accept as reliable (start with about 0.45)
*/
func NewSigCurve(numsamples int, mindatapoints int, minslope float64, window int, minrsqrd float64) *SigCurve {
	if mindatapoints >= numsamples {
		log.Panic("Splitpoint must be less than num data samples ", mindatapoints, ">=", numsamples)
	}
	if window >= numsamples-mindatapoints {
		log.Panic("The window should be much smaller than the difference between the number of samples and the mindatapoints")
	}

	return &SigCurve{
		variance:             managedslice.NewManagedSlice(0, numsamples),            ///we only need 2 * window here - but keep the rest for now for debug
		variancetime:         managedslice.NewManagedSlice(0, numsamples),            ///do LR against time on the x axis
		variancecurve:        managedslice.NewManagedSlice(0, (numsamples/window)+1), ///we need numsamples / window
		variancecurvedbg:     managedslice.NewManagedSlice(0, numsamples),            /// for printing
		rsqrd:                managedslice.NewManagedSlice(0, numsamples),            /// for printing
		mindatapoints:        (mindatapoints / window) + 1,                           /// divde by the window to get it in block averages
		numorderbooksamples:  numsamples,
		minslope:             minslope,
		statsvariancebuysig:  perfstats.NewCounter("variance-buy-signalled"),
		statsvariancesellsig: perfstats.NewCounter("variance-buy-signalled"),
		window:               window,
		wndcounter:           0,
		averagewndsize:       ((numsamples - mindatapoints) / window) + 1,
		minrsqrd:             minrsqrd,
		statslopedata:        perfstats.NewBucketCounter(-100, 100, 4, "slope-stats"),
		statrsqrddata:        perfstats.NewBucketCounter(-1, 1, 0.05, "rsqrd-stats"),
		statsvaliddata:       perfstats.NewCounter("valid-data-sample"),
	}
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
	firstsampletime := sampletime[0].(time.Time)
	y := make([]float64, len(data))
	x := make([]float64, len(data))
	timerange := sampletime[len(sampletime)-1].(time.Time).Sub(sampletime[0].(time.Time))
	avgtime := float64(timerange) / float64(len(sampletime))
	for i, _ := range data {
		x[i] = float64(sampletime[i].(time.Time).Sub(firstsampletime)) / avgtime
		y[i] = data[i].(float64)
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
		//see if the last item is a non-shallow upward curve
		if p.variancecurve.FromBack(0).(float64) > 0 {
			angle := p.variancecurve.FromBack(0).(float64)
			p.statslopedata.Inc(angle)
			if angle > p.minslope {
				return true, true
			}
		} else if p.variancecurve.FromBack(0).(float64) < 0 {
			angle := p.variancecurve.FromBack(0).(float64)
			p.statslopedata.Inc(angle)
			if math.Abs(angle) > p.minslope {
				return true, false
			}
		}
	} else {
		fmt.Println("Not enough data points ", p.variancecurve.Len(), p.mindatapoints)
	}
	return false, false
}

func (p *SigCurve) AddVarianceSample(variance float64, t time.Time) {
	p.variance.PushAndResize(variance)
	p.variancetime.PushAndResize(t)
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
			p.rsqrd.PushAndResize(rsqrd)
		}
		/// don't push dubious results
		if rsqrd > p.minrsqrd {
			if math.IsNaN(grad) {
				log.Panicln("Grad is NaN ", grad)
			}
			//p.logdbg("adding LR data ", grad)
			p.variancecurve.PushAndResize(grad)
			p.variancecurvedbg.PushAndResize(grad)
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
