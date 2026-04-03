package pricing

import "testing"

func TestCalcCost_Basic(t *testing.T) {
	// prices: [input, output, cache_read, cache_creation]
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	cost := CalcCost(1000, 500, 0, 0, prices)
	// 1000 * 0.003 + 500 * 0.015 = 3.0 + 7.5 = 10.5
	if cost != 10.5 {
		t.Errorf("expected 10.5, got %f", cost)
	}
}

func TestCalcCost_WithCache(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	// input=1000, output=500, cacheCreation=200, cacheRead=300
	// regularInput = 1000 - 300 - 200 = 500
	// cost = 500*0.003 + 200*0.004 + 300*0.001 + 500*0.015
	//      = 1.5 + 0.8 + 0.3 + 7.5 = 10.1
	cost := CalcCost(1000, 500, 200, 300, prices)
	expected := 10.1
	if diff := cost - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCalcCost_NegativeRegularInput(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	// cacheRead + cacheCreation > input → regularInput clamped to 0
	cost := CalcCost(100, 500, 200, 300, prices)
	// regularInput = max(100-300-200, 0) = 0
	// cost = 0 + 200*0.004 + 300*0.001 + 500*0.015 = 0.8 + 0.3 + 7.5 = 8.6
	expected := 8.6
	if diff := cost - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCalcCost_ZeroTokens(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}
	cost := CalcCost(0, 0, 0, 0, prices)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}

func TestCalcCost_ZeroPrices(t *testing.T) {
	prices := [4]float64{0, 0, 0, 0}
	cost := CalcCost(1000, 500, 200, 300, prices)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}
