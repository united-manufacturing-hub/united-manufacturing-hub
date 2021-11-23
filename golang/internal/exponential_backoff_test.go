package internal

import (
	"testing"
	"time"
)

func TestGetBackoffTime(t *testing.T) {
	for i := int64(0); i < 1024; i++ {
		max := 10 * time.Second
		btime := GetBackoffTime(i, 1*time.Nanosecond, max)
		if btime < 0 || btime > max {
			t.Logf("BTime: %s", btime)
			t.Logf("I: %d", i)
			t.FailNow()
		}
	}
}

func TestPlotBackoffTime(t *testing.T) {
	/*
		fname := ""
		persist := false
		debug := true

		p, err := gnuplot.NewPlotter(fname, persist, debug)
		if err != nil {
			err_string := fmt.Sprintf("** err: %v\n", err)
			panic(err_string)
		}
		defer p.Close()

		p.PlotX([]float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "some data")
		p.CheckedCmd("set terminal pdf")
		p.CheckedCmd("set output 'plot002.pdf'")
		p.CheckedCmd("replot")

		p.CheckedCmd("q")

	*/
}
