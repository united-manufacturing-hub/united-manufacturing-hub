package internal

type Xy struct {
	X []float64
	Y []float64
}

func (d Xy) Len() int {
	return len(d.X)
}

func (d Xy) XY(i int) (x, y float64) {
	x = d.X[i]
	y = d.Y[i]
	return
}
