package signals

import (
	"fmt"
	"github.com/paul-at-nangalan/signals/store"
	"gotest.tools/v3/assert"
	"math"
	"math/rand"
	"testing"
	"time"
)

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func TestOrderBookVariance_AddVarianceSample(t *testing.T) {
	numsamples := 1000
	split := 900
	window := 10
	minslope := 0.35
	minrsqrd := 0.45
	obv := NewSigCurve(numsamples, split, minslope, window, minrsqrd)
	obv.loglevel = LOGDBG
	extra := 5

	vals := make([]float64, 0)

	/// test a downturn - use sin to generate
	startvalue := float64(1322) /// any number
	for i := 0; i < numsamples+extra; i++ {
		rad := (float64(i)/float64(numsamples))*(math.Pi/2) +
			((float64(i) / float64(numsamples)) * (math.Pi * ((float64(numsamples) - float64(split)) / float64(numsamples))))
		value := math.Sin(rad) * startvalue
		obv.AddVarianceSample(value, time.Now())
		/// this should never trigger a buy signal
		if obv.SigBuy() {
			t.Error("Buy signalled for a downturn")
		}
		if obv.SigSell() {
			fmt.Println("Sell signalled at ", i)
		}
		vals = append(vals, value)
	}
	obv.Plot()
	//for _, f := range obv.variancecurvedbg.Items() {
	//	fmt.Print(f, ", ")
	//}
	fmt.Println()
	if !obv.SigSell() {
		t.Error("Failed to signal sell on downturn")
	}

	/// test flat upwards flat
	/*
		     _____
		    /
		___/
	*/
	vals = make([]float64, 0)
	obv = NewSigCurve(numsamples, split, minslope, window, minrsqrd)

	sigmoidrange := float64(10)
	sigmoidstart := float64(-5)
	for i := 0; i < numsamples+numsamples+extra; i++ {
		x := ((float64(i) / float64(numsamples)) * sigmoidrange) + sigmoidstart
		value := sigmoid(x) * startvalue
		obv.AddVarianceSample(value, time.Now())
		/// this should never trigger a buy signal

		vals = append(vals, value)
	}
	if obv.SigBuy() {
		t.Error("Buy signalled for sigmoid")
	}
	if obv.SigSell() {
		t.Error("Sell signalled for sigmoid")
	}
	obv.Plot()

	vals = make([]float64, 0)
	///Test an upturn - this should trigger at the end
	obv = NewSigCurve(numsamples, split, minslope, window, minrsqrd)
	for i := 0; i < numsamples+extra; i++ {
		rad := (float64(i)/float64(numsamples))*(math.Pi/2) +
			((float64(i) / float64(numsamples)) * (math.Pi * ((float64(numsamples) - float64(split)) / float64(numsamples)))) + math.Pi
		value := (1 + math.Sin(rad)) * startvalue
		obv.AddVarianceSample(value, time.Now())
		/// this should never trigger a buy signal
		if i < int(float64(split)/float64(numsamples)) && obv.SigBuy() {
			t.Error("Buy signalled too early")
		}
		vals = append(vals, value)
	}
	obv.Plot()
	if !obv.SigBuy() {
		t.Error("Failed to signal buy on upturn test")
	}

	vals = make([]float64, 0)
	///Test random data - should not trigger either signal
	obv = NewSigCurve(numsamples, split, minslope, window, minrsqrd)
	for i := 0; i < numsamples+extra; i++ {
		value := rand.Float64()
		obv.AddVarianceSample(value, time.Now())
		/// this should never trigger a buy signal
		if obv.SigSell() || obv.SigBuy() {
			t.Error("Buy/Sell signalled for random data - try re-running as sometimes random data can generate signals")
		}
		vals = append(vals, value)
	}
	obv.Plot()
}

func Test_StoreAndRetrieve(t *testing.T) {
	numsamples := 1000
	split := 900
	window := 10
	minslope := 0.35
	minrsqrd := 0.45
	obv := NewSigCurve(numsamples, split, minslope, window, minrsqrd)
	obv.loglevel = LOGDBG
	extra := 5

	datastore := store.NewFileStore("/tmp/test-curve")
	obv.SetupStorage("test-curve", datastore, time.Millisecond)

	lastrad := float64(0)
	startvalue := float64(1322) /// any number
	for i := 0; i < numsamples+extra; i++ {
		rad := (float64(i)/float64(numsamples))*(math.Pi/2) +
			((float64(i) / float64(numsamples)) * (math.Pi * ((float64(numsamples) - float64(split)) / float64(numsamples))))
		value := math.Sin(rad) * startvalue
		obv.AddVarianceSample(value, time.Now())
		/// this should never trigger a buy signal
		if obv.SigBuy() {
			t.Error("Buy signalled for a downturn")
		}
		lastrad = rad
	}
	obv.Plot()
	//for _, f := range obv.variancecurvedbg.Items() {
	//	fmt.Print(f, ", ")
	//}
	fmt.Println()
	if !obv.SigSell() {
		t.Error("Failed to signal sell on downturn")
	}
	/// make sure it done a save of the last data
	obv.storeData()
	//// Load it back from stored data
	sig, isvalid := LoadFromStorage("test-curve", datastore, time.Hour)
	assert.Equal(t, isvalid, true, "SigCurve invalid after reload")
	/// add one more value to trigger the sig sell
	sig.AddVarianceSample(lastrad, time.Now())
	sig.Plot()
	assert.Equal(t, sig.SigSell(), true, "Mismatch - expected sig sell to be signalled after reload")
}
