package bloom

import (
	"testing"
)

func TestParams1(t *testing.T) {
	m, k, err := Params(1000000, 0.01)
	if err != nil {
		t.Fatalf("Params вернул ошибку: %v", err)
	}
	if m != 9585059 || k != 7 {
		t.Errorf("При n=100000, p=0,01 ожидалось m=9585059, k=7, а получилось m=%v и k=%v", m, k)
	}
}

func TestParams2(t *testing.T) {
	m, k, err := Params(100, 0.01)
	if err != nil {
		t.Fatalf("Params вернул ошибку: %v", err)
	}
	if m != 959 || k != 7 {
		t.Errorf("При n=100, p=0,01 ожидалось m=959, k=7, а получилось m=%v и k=%v", m, k)
	}
}

func TestParams3(t *testing.T) {
	m, k, err := Params(500000, 0.001)
	if err != nil {
		t.Fatalf("Params вернул ошибку: %v", err)
	}
	if m != 7188794 || k != 10 {
		t.Errorf("При n=500000, p=0,001 ожидалось m=7188794, k=10, а получилось m=%v и k=%v", m, k)
	}
}

func TestParams4(t *testing.T) {
	m, k, err := Params(1000, 0.1)
	if err != nil {
		t.Fatalf("Params вернул ошибку: %v", err)
	}
	if m != 4793 || k != 3 {
		t.Errorf("При n=1000, p=0,1 ожидалось m=4793, k=3, а получилось m=%v и k=%v", m, k)
	}
}

func TestParams5(t *testing.T) {
	m, k, err := Params(10, 0.5)
	if err != nil {
		t.Fatalf("Params вернул ошибку: %v", err)
	}
	if m != 15 || k != 1 {
		t.Errorf("При n=10, p=0,5 ожидалось m=15, k=1, а получилось m=%v и k=%v", m, k)
	}
}

func TestParamsNegativeN(t *testing.T) {
	_, _, err := Params(-100, 0.01)
	if err == nil {
		t.Errorf("ожидали ошибку при отрицательном n")
	}
}

func TestParamsZeroN(t *testing.T) {
	_, _, err := Params(0, 0.01)
	if err == nil {
		t.Errorf("ожидали ошибку при n = 0")
	}
}

func TestParamsZeroRate(t *testing.T) {
	_, _, err := Params(1000, 0)
	if err == nil {
		t.Errorf("ожидали ошибку при p=0")
	}
}

func TestParamsRateMoreOne(t *testing.T) {
	_, _, err := Params(1000, 1.5)
	if err == nil {
		t.Errorf("ожидали ошибку при p=1.5")
	}
}
