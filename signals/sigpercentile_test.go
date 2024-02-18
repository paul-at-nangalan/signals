package signals

import (
	"github.com/paul-at-nangalan/signals/store"
	"gonum.org/v1/gonum/stat/distuv"
	"gotest.tools/v3/assert"
	"testing"
	"time"
)

func genNormalDist(size int, lower, upper float64) []float64 {

	// Define the normal distribution parameters
	mean := 0.0   // Adjust this to change the mean
	stddev := 1.0 // Adjust this to change the standard deviation
	dist := distuv.Normal{
		Mu:    mean,
		Sigma: stddev,
	}

	datapoints := make([]float64, size)
	// Generate random numbers
	value := dist.Rand()
	min := value
	max := value
	for i := 0; i < size; i++ {
		value := dist.Rand()
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
		datapoints[i] = value
	}
	/// remap into our range
	for i, dp := range datapoints {
		transval := (((dp - min) / (max - min)) * (upper - lower)) + lower
		datapoints[i] = transval
	}
	return datapoints
}

func checkPC(sig *SigPercentile, val, expabove, expbelow float64, t *testing.T) {
	pc := sig.checkData(val)
	///I'd expect this to be below 0.25
	if !(pc >= expabove && pc <= expbelow) {
		sig.Plot()
		t.Error("Percentile value seems incorrect ", pc, val, expabove, expbelow, sig.lower, sig.upper)
	}
}

func fillSig(sig *SigPercentile, size int, lower, upper float64) {
	vals := genNormalDist(size, lower, upper)
	for _, val := range vals {
		sig.AddData(val)
	}
}

func TestSigPercentile_Percentile(t *testing.T) {
	lower := 100.0
	upper := 200.0
	sig := NewSigPercentile(0.25, 0.75, 1000, 2*time.Second)
	fillSig(sig, 3000, lower, upper)
	checkPC(sig, 120, 0, 0.25, t)
	checkPC(sig, 150, 0.25, 0.75, t) /// it seems we can't make this too tight
	checkPC(sig, 175, 0.75, 1.0, t)

}

func TestSigPercentile_ShiftRange(t *testing.T) {
	lower := 100.0
	upper := 200.0
	sig := NewSigPercentile(0.25, 0.75, 1000, 2*time.Second)
	fillSig(sig, 3000, lower, upper)

	checkPC(sig, 120, 0, 0.25, t)
	checkPC(sig, 150, 0.25, 0.75, t)
	checkPC(sig, 175, 0.75, 1.0, t)
	time.Sleep(3 * time.Second)
	///shift the range down
	// and fill
	lower = 90
	upper = 190
	fillSig(sig, 3000, lower, upper)
	sig.prune()
	checkPC(sig, 100, 0, 0.25, t)
	checkPC(sig, 140, 0.25, 0.75, t)
	checkPC(sig, 180, 0.75, 1.0, t)
	time.Sleep(3 * time.Second)

	///now shift up
	lower = 110
	upper = 210
	fillSig(sig, 3000, lower, upper)
	sig.prune()
	checkPC(sig, 120, 0, 0.25, t)
	checkPC(sig, 160, 0.25, 0.75, t)
	checkPC(sig, 200, 0.75, 1, t)
}

func TestSigPercentile_prune(t *testing.T) {
	lower := 100.0
	upper := 200.0
	sig := NewSigPercentile(0.25, 0.75, 1000, 2*time.Second)
	fillSig(sig, 3000, lower, upper)
	time.Sleep(3 * time.Second)
	fillSig(sig, 3000, lower+12, upper-22)
	sig.prune()
	for _, bin := range sig.bins {
		/// we shouldnt see any bins with a upper value below lower+12 or lower value above upper - 22
		if bin.upperval < lower+12 {
			t.Error("Prune failed to clear out the lower ranges ", bin)
		}
		if bin.lowerval > upper-22 {
			t.Error("Prune failed to clear out upper ranges ", bin)
		}
	}
}

func Test_StoreAndRestore(t *testing.T) {
	lower := 100.0
	upper := 200.0
	sig := NewSigPercentile(0.25, 0.75, 1000, 2*time.Second)
	fs := store.NewFileStore("/tmp/test-percentile")
	sig.SetupStorage("sig-percentile", fs, time.Millisecond)
	fillSig(sig, 3000, lower, upper)
	checkPC(sig, 120, 0, 0.25, t)
	checkPC(sig, 150, 0.25, 0.75, t) /// it seems we can't make this too tight
	checkPC(sig, 175, 0.75, 1.0, t)
	time.Sleep(2 * time.Second) ///make sure it's had time to save
	///add one more data point
	sig.AddData((upper - lower) / 2) ///this should force a save

	loaded, isvalid := LoadFromStorageSigPC("sig-percentile", fs, time.Hour)
	assert.Equal(t, isvalid, true, "Failed to load sig percentile from storage ")
	checkPC(loaded, 120, 0, 0.25, t)
	checkPC(loaded, 150, 0.25, 0.75, t) /// it seems we can't make this too tight
	checkPC(loaded, 175, 0.75, 1.0, t)

}
