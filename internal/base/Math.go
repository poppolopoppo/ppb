package base

func Dot(a, b []float64) (result float64) {
	for i, ai := range a {
		result += ai * b[i]
	}
	return
}

func Lerp(a, b float64, f float64) float64 {
	return (a + (b-a)*Saturate(f))
}

func Smootherstep(x float64) float64 {
	x = Saturate(x)
	return x * x * x * (x*(x*6.0-15.0) + 10.0)
}

func Saturate(x float64) float64 {
	if x > 1 {
		return 1
	} else if x < 0 {
		return 0
	} else {
		return x
	}
}

/***************************************
 * Moving Average
 ***************************************/

// Double exponential smoothing (Holt linear)
// https://en.wikipedia.org/wiki/Exponential_smoothing

const (
	MOVINGAVG_DATA_SMOOTHING = 0.1
)

type MovingAverage struct {
	dx  float64
	ddx float64
}

func NewMovingAverage(init float64) MovingAverage {
	return MovingAverage{
		dx:  init,
		ddx: init,
	}
}

// Brown's linear exponential smoothing (LES)
func (x *MovingAverage) Reset(f float64) {
	x.dx = f
	x.ddx = f
}
func (x *MovingAverage) Add(f float64) {
	x.dx = MOVINGAVG_DATA_SMOOTHING*f + (1-MOVINGAVG_DATA_SMOOTHING)*x.dx
	x.ddx = MOVINGAVG_DATA_SMOOTHING*x.dx + (1-MOVINGAVG_DATA_SMOOTHING)*x.ddx
}
func (x *MovingAverage) Majorate(f float64) {
	x.Add(f)
	if x.EstimatedLevel() < f {
		x.Reset(f)
	}
}
func (x *MovingAverage) EstimatedLevel() float64 {
	return 2*x.dx - x.ddx
}
func (x *MovingAverage) EstimatedTrend() float64 {
	return MOVINGAVG_DATA_SMOOTHING / (1 - MOVINGAVG_DATA_SMOOTHING) * (x.dx - x.ddx)
}
func (x *MovingAverage) Forecast(m float64) float64 {

	return x.EstimatedLevel() + m*x.EstimatedTrend()
}
