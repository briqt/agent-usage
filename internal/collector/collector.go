package collector

// Collector is the interface that all data source collectors implement.
type Collector interface {
	Scan() error
}
