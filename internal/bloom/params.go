package bloom

import (
	"fmt"
	"math"
)

// Вычисление m и k
func Params(n int, p float64) (int, int, error) {
	if n <= 0 {
		return 0, 0, fmt.Errorf("n не может быть меньше 0")
	}
	if p <= 0 || p > 1 {
		return 0, 0, fmt.Errorf("p от 0 до 1")
	}
	m := int(math.Ceil((float64(-n) * math.Log(p)) / (math.Pow(math.Ln2, 2))))
	k := int(math.Round(float64(m) * math.Ln2 / float64(n)))
	return m, k, nil
}
