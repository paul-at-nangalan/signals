package dataplot

import (
	"fmt"
	"log"
	"math"
	"signals/managedslice"
)

func PlotManagedSlice(data *managedslice.Slice, vx, vy int) {
	datapoints := make([]float64, data.Len())
	for i := 0; i < len(datapoints); i++ {
		datapoints[i] = data.At(i).(float64)
	}
	Plot(datapoints, vx, vy)
}

func Plot(vals []float64, vx, vy int) {
	if len(vals) < 2 {
		return
	}
	min := vals[0]
	max := vals[0]
	for _, val := range vals {
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}
	if min == 0 && max == 0 {
		fmt.Println("Cannot print data points, all are zero")
		return
	}
	yaxis := make([]string, vy)
	intrvl := (max - min) / float64(vy)
	for i := 0; i < vy; i++ {
		yaxis[i] = fmt.Sprintf("	%+9.4f %s", min+(intrvl*float64(i)), "|")
	}
	xaxis := fmt.Sprintf("		0 			-			 %+9.4f", vals[len(vals)-1])

	display := make([][]byte, vy)
	for i, _ := range display {
		display[i] = make([]byte, vx)
		for x, _ := range display[i] {
			display[i][x] = byte(' ')
		}
	}
	for i := 0; i < len(vals); i++ {
		transval := ((vals[i] - min) / (max - min)) * float64(len(display)-1)
		y := math.Round(transval)
		transval = (float64(i) / float64(len(vals))) * float64(len(display[0])-1)
		x := math.Round(transval)
		if int(x) > len(display[0]) || int(y) > len(display) || int(x) < 0 || int(y) < 0 {
			log.Panic("Somethings out of bounds ", int(x), " > ", len(display[0]), " || ", int(y), " > ", len(display),
				" for ", vals[i], " and ", i, " min/max ", min, max)
		}
		display[int(y)][int(x)] = byte('x')
	}
	for i := 0; i < len(display); i++ {
		fmt.Println(yaxis[len(display)-(i+1)], string(display[len(display)-(i+1)]))
	}
	fmt.Println("\t", xaxis)

	fmt.Println()
	//fmt.Println(vals)
}
