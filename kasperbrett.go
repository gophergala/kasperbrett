package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/boltdb/bolt"
	"github.com/nu7hatch/gouuid"
	"github.com/robertkrimen/otto"
	"gopkg.in/tomb.v2"
	"net/http"
	"time"
)

func main() {
	fmt.Println("Hello Kasperbrett!")
}

type SocketIOApi interface {
	Handler() http.Handler
	Broadcast(*Sample)
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

/* ***** ██████╗  █████╗ ████████╗ █████╗     ███████╗████████╗ ██████╗ ██████╗  █████╗  ██████╗ ███████╗ ***** */
/* ***** ██╔══██╗██╔══██╗╚══██╔══╝██╔══██╗    ██╔════╝╚══██╔══╝██╔═══██╗██╔══██╗██╔══██╗██╔════╝ ██╔════╝ ***** */
/* ***** ██║  ██║███████║   ██║   ███████║    ███████╗   ██║   ██║   ██║██████╔╝███████║██║  ███╗█████╗   ***** */
/* ***** ██║  ██║██╔══██║   ██║   ██╔══██║    ╚════██║   ██║   ██║   ██║██╔══██╗██╔══██║██║   ██║██╔══╝   ***** */
/* ***** ██████╔╝██║  ██║   ██║   ██║  ██║    ███████║   ██║   ╚██████╔╝██║  ██║██║  ██║╚██████╔╝███████╗ ***** */
/* ***** ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝    ╚══════╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚══════╝ ***** */
/* ***** Banner comment generated by http://patorjk.com/software/taag ***** ***** ***** ***** ***** ***** ***** */

// TODO: Refactor this section to a separate package!
// TODO: Add test cases

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type DataStore interface {
	Prepare() error
	ShutDown() error
	PersistSamples(samples []*Sample) error
	GetSamples(dataSourceId string, from time.Time, to time.Time) ([]*Sample, error)
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

const (
	BoltDataFileName       = "kasperbrett.db"
	BoltSamplesBucket      = "KasperbrettSamples"
	BoltSampleKeySeparator = "#"
)

func NewBoltDataStore(dataFileAbsPath string) *BoltDataStore {
	return &BoltDataStore{dataFileAbsPath: dataFileAbsPath}
}

type BoltDataStore struct {
	dataFileAbsPath string
	db              *bolt.DB
}

func (ds *BoltDataStore) Prepare() error {
	fmt.Println("[BoltDataStore] Prepare()")
	db, err := bolt.Open(ds.dataFileAbsPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	if err != nil {
		return err
	}

	ds.db = db

	return ds.createBucketIfNotExists(BoltSamplesBucket)
}

func (ds *BoltDataStore) ShutDown() error {
	fmt.Println("[BoltDataStore] ShutDown()")
	return ds.db.Close()
}

func (ds *BoltDataStore) PersistSamples(samples []*Sample) error {
	return ds.db.Update(func(tx *bolt.Tx) error {
		fmt.Printf("[BoltDataStore] Persisting %d samples\n", len(samples))
		b := tx.Bucket([]byte(BoltSamplesBucket))

		var overallError error = nil

		for _, sample := range samples {
			fmt.Printf("[BoltDataStore] Persisting sample %s (%s)\n", sample.Key(), sample.String())
			sampleBytes, err := sample.GobEncode()
			if err != nil {
				if overallError == nil {
					overallError = err
				}
			} else {
				err := b.Put([]byte(sample.Key()), sampleBytes)
				if err != nil && overallError == nil {
					overallError = err
				}
			}
		}

		return overallError
	})
}

func (ds *BoltDataStore) GetSamples(dataSourceId string, from time.Time, to time.Time) ([]*Sample, error) {
	samples := []*Sample{}

	err := ds.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(BoltSamplesBucket)).Cursor()
		min := []byte(GenerateKey(dataSourceId, BoltSampleKeySeparator, from))
		max := []byte(GenerateKey(dataSourceId, BoltSampleKeySeparator, to))

		fmt.Printf(" [BoltDataStore.GetSamples()] from -> %s ||| to -> %s\n", from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))

		var err error
		var sample *Sample
		for k, sampleBytes := c.Seek(min); k != nil && bytes.Compare(k, max) <= 0; k, sampleBytes = c.Next() {
			sample = new(Sample)
			err = sample.GobDecode(sampleBytes)
			if err != nil {
				fmt.Printf("[BoltDataStore.GetSamples()] Couldn't read sample %s due to: %s\n", k, err.Error())
			} else {
				samples = append(samples, sample)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	} else {
		return samples, nil
	}
}

func (ds *BoltDataStore) createBucketIfNotExists(bucketName string) error {
	return ds.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

/* ***** ██████╗  █████╗ ████████╗ █████╗     ██████╗ ███████╗████████╗██████╗ ██╗███████╗██╗   ██╗ █████╗ ██╗      ***** */
/* ***** ██╔══██╗██╔══██╗╚══██╔══╝██╔══██╗    ██╔══██╗██╔════╝╚══██╔══╝██╔══██╗██║██╔════╝██║   ██║██╔══██╗██║      ***** */
/* ***** ██║  ██║███████║   ██║   ███████║    ██████╔╝█████╗     ██║   ██████╔╝██║█████╗  ██║   ██║███████║██║      ***** */
/* ***** ██║  ██║██╔══██║   ██║   ██╔══██║    ██╔══██╗██╔══╝     ██║   ██╔══██╗██║██╔══╝  ╚██╗ ██╔╝██╔══██║██║      ***** */
/* ***** ██████╔╝██║  ██║   ██║   ██║  ██║    ██║  ██║███████╗   ██║   ██║  ██║██║███████╗ ╚████╔╝ ██║  ██║███████╗ ***** */
/* ***** ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝    ╚═╝  ╚═╝╚══════╝   ╚═╝   ╚═╝  ╚═╝╚═╝╚══════╝  ╚═══╝  ╚═╝  ╚═╝╚══════╝ ***** */
/* ***** Banner comment generated by http://patorjk.com/software/taag ***** ***** ***** ***** ***** ***** ***** *** ***** */

// TODO: Refactor this section to a separate package!
// TODO: Add test cases

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type DataSource interface {
	Retrieve(sampleChan chan *Sample)
	Id() string
	Type() string
	Name() string
	Interval() time.Duration
}

const (
	DsUrlScraper = "DsUrlScraper"
)

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func RetrieveAndDistribute(ds DataSource, re ReportingEngine, timeout time.Duration) {
	sampleChan := make(chan *Sample)
	go ds.Retrieve(sampleChan)

	var sample *Sample

	select {
	case sample = <-sampleChan:
	case now := <-time.After(timeout):
		sample = NewSample("", now, ds.Id(), errors.New("Sample retrieval timed out."))
	}

	re.Distribute(sample)
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewAbstractDataSource(dataSourceId string) AbstractDataSource {
	return AbstractDataSource{dataSourceId: dataSourceId}
}

type AbstractDataSource struct {
	dataSourceId string
	name         string
	interval     time.Duration
}

func (this AbstractDataSource) Id() string {
	return this.dataSourceId
}

func (this AbstractDataSource) Name() string {
	return this.name
}

func (this AbstractDataSource) Interval() time.Duration {
	return this.interval
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func GenerateKey(dataSourceId string, keySeparator string, timestamp time.Time) string {
	// Due to the characteristics of Bolt (it stores keys in byte-sorted order) we use a time format that is sortable (RFC3339).
	// Additionally, when it comes to time we want to be as precise as possible.
	// And we need to avoid key collisions in case a data source has a very short retrieval interval (< 1s).
	// Therefore we use RFC3339Nano instead of simple RFC3339.
	// Furthermore, bucket keys should be based on reference time instead of local time. That's why we choose UTC.
	return fmt.Sprintf("%s%s%s", dataSourceId, keySeparator, timestamp.UTC().Format(time.RFC3339Nano))
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewSample(value string, timestamp time.Time, dataSourceId string, err error) *Sample {
	if len(dataSourceId) == 0 {
		panic("A Sample must have a valid dataSourceId.")
	}

	if len(value) == 0 && err == nil {
		panic("A Sample must have a valid value or a specified error.")
	}

	return &Sample{
		Value:        value,
		Timestamp:    timestamp,
		DataSourceId: dataSourceId,
		Err:          err,
	}
}

type Sample struct {
	Value        string
	Timestamp    time.Time
	DataSourceId string
	Err          error
}

func (this *Sample) JSON() string {
	b, err := json.Marshal(this)
	if err != nil {
		// TODO: log this misbehaviour
		return "{}"
	}

	return string(b)
}

func (this *Sample) Key() string {
	return GenerateKey(this.DataSourceId, BoltSampleKeySeparator, this.Timestamp)
}

func (this *Sample) GobEncode() ([]byte, error) {
	// TODO: It might make sense to include a version number in the encoding due to future changes.

	buff := new(bytes.Buffer)
	encoder := gob.NewEncoder(buff)

	err := encoder.Encode(this.Value)
	if err != nil {
		return nil, err
	}

	err = encoder.Encode(this.Timestamp)
	if err != nil {
		return nil, err
	}

	err = encoder.Encode(this.DataSourceId)
	if err != nil {
		return nil, err
	}

	errStr := ""
	if this.Err != nil {
		errStr = this.Err.Error()
	}

	err = encoder.Encode(errStr)
	if err != nil {
		return nil, err
	}

	return buff.Bytes(), nil
}

func (this *Sample) GobDecode(sampleBytes []byte) error {
	// TODO: It might make sense to include a version number in the encoding due to future changes.
	fmt.Println("   [Sample]   GobDecode()")

	buff := bytes.NewBuffer(sampleBytes)
	decoder := gob.NewDecoder(buff)

	err := decoder.Decode(&this.Value)
	if err != nil {
		return err
	}

	err = decoder.Decode(&this.Timestamp)
	if err != nil {
		return err
	}

	err = decoder.Decode(&this.DataSourceId)
	if err != nil {
		return err
	}

	var str string
	err = decoder.Decode(&str)
	if err != nil {
		return err
	}

	if len(str) > 0 {
		this.Err = errors.New(str)
	} else {
		this.Err = nil
	}

	return nil
}

func (this *Sample) String() string {
	if len(this.Value) > 0 {
		return fmt.Sprintf("[%s] -> %s", this.Timestamp.UTC().Format(time.RFC1123Z), this.Value)
	} else {
		return fmt.Sprintf("[%s] -> %s", this.Timestamp.UTC().Format(time.RFC1123Z), this.Err)
	}
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewUrlScraper(url string, cssPath string, transformationScript string) (*UrlScraper, error) {
	uuid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}

	dataSourceId := "ds-" + uuid.String()

	return &UrlScraper{
		AbstractDataSource:   NewAbstractDataSource(dataSourceId),
		url:                  url,
		cssPath:              cssPath,
		jsEngine:             otto.New(),
		transformationScript: transformationScript,
	}, nil
}

type UrlScraper struct {
	AbstractDataSource
	url                  string
	cssPath              string
	jsEngine             *otto.Otto
	transformationScript string
}

func (this *UrlScraper) Retrieve(sampleChan chan *Sample) {
	var sample *Sample
	t := time.Now()
	doc, err := goquery.NewDocument(this.url)
	if err != nil {
		sample = NewSample("", t, this.dataSourceId, err)
	}

	value := doc.Find(this.cssPath).Text()
	if len(value) == 0 {
		sample = NewSample("", t, this.dataSourceId, errors.New("The specified CSS path is invalid or doesn't match any DOM nodes."))
	}

	if len(this.transformationScript) > 0 {
		// TODO: perform some JS sanitation to prevent injection of harmful JS code
		_, err = this.jsEngine.Run("var value = '" + value + "';")
		if err != nil {
			sample = NewSample("", t, this.dataSourceId, err)
		}

		jsValue, err := this.jsEngine.Run("value = " + this.transformationScript + ";")
		if err != nil {
			sample = NewSample("", t, this.dataSourceId, err)
		}

		value = jsValue.String()
		if len(value) == 0 {
			sample = NewSample("", t, this.dataSourceId, errors.New("Couldn't perform the provided JS transformation."))
		}
	}

	sample = NewSample(value, t, this.dataSourceId, nil)
	sampleChan <- sample
}

func (this *UrlScraper) Type() string {
	return DsUrlScraper
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

/* ***** ██████╗ ███████╗██████╗  ██████╗ ██████╗ ████████╗██╗███╗   ██╗ ██████╗  ***** */
/* ***** ██╔══██╗██╔════╝██╔══██╗██╔═══██╗██╔══██╗╚══██╔══╝██║████╗  ██║██╔════╝  ***** */
/* ***** ██████╔╝█████╗  ██████╔╝██║   ██║██████╔╝   ██║   ██║██╔██╗ ██║██║  ███╗ ***** */
/* ***** ██╔══██╗██╔══╝  ██╔═══╝ ██║   ██║██╔══██╗   ██║   ██║██║╚██╗██║██║   ██║ ***** */
/* ***** ██║  ██║███████╗██║     ╚██████╔╝██║  ██║   ██║   ██║██║ ╚████║╚██████╔╝ ***** */
/* ***** ╚═╝  ╚═╝╚══════╝╚═╝      ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝╚═╝  ╚═══╝ ╚═════╝  ***** */
/* ***** Banner comment generated by http://patorjk.com/software/taag ***** ***** ***** */

// TODO: Refactor this section to a separate package!
// TODO: Add test cases

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type Reporter interface {
	OnSample(sample *Sample)
	Prepare() error
	ShutDown() error
}

type ReportingEngine interface {
	Register(reporters ...Reporter)
	Distribute(sample *Sample)
	ShutDown() error
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewSocketIOReporter(socketIOApi SocketIOApi) SocketIOReporter {
	return SocketIOReporter{socketIOApi: socketIOApi}
}

type SocketIOReporter struct {
	socketIOApi SocketIOApi
}

func (r SocketIOReporter) OnSample(sample *Sample) {
	r.socketIOApi.Broadcast(sample)
}

func (r SocketIOReporter) Prepare() error {
	return nil
}

func (r SocketIOReporter) ShutDown() error {
	return nil
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewConsoleReporter(prefix string) ConsoleReporter {
	return ConsoleReporter{prefix: prefix}
}

type ConsoleReporter struct {
	prefix string
}

func (r ConsoleReporter) OnSample(sample *Sample) {
	fmt.Printf("%s%s\n", r.prefix, sample)
}

func (r ConsoleReporter) Prepare() error {
	return nil
}

func (r ConsoleReporter) ShutDown() error {
	return nil
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewPersistentDataStoreReporter(dataStore DataStore, flushInterval time.Duration) *PersistentDataStoreReporter {
	r := &PersistentDataStoreReporter{
		dataStore:         dataStore,
		flushTicker:       time.NewTicker(flushInterval),
		sampleChan:        make(chan *Sample),
		sampleRequestChan: make(chan SampleRetrievalRequest),
	}

	go func() {
		for {
			select {
			case <-r.flushTicker.C:
				samplesToFlush := r.buffer[0:len(r.buffer)]
				err := r.dataStore.PersistSamples(samplesToFlush)
				if err != nil {
					fmt.Println("[PersistentDataStoreReporter] Couldn't persist samples due to:", err)
				}

				// clear buffer
				// Attention! This might produce a memory leak...
				// see: http://stackoverflow.com/questions/16971741/how-do-you-clear-a-slice-in-go
				r.buffer = r.buffer[:0]

			case sample := <-r.sampleChan:
				// it might be better to impl. a custom append() function for performance reasons
				// but in the first version the built-in one is sufficient
				r.buffer = append(r.buffer, sample)

			case sampleRetrievalRequest := <-r.sampleRequestChan:
				var eligibleSamples []*Sample

				for _, sample := range r.buffer {
					doesDataSourceIdMatch := sample.DataSourceId == sampleRetrievalRequest.DataSourceId
					doesFromTimeMatch := sample.Timestamp.Equal(sampleRetrievalRequest.From) || sample.Timestamp.After(sampleRetrievalRequest.From)
					doesToTimeMatch := sample.Timestamp.Equal(sampleRetrievalRequest.To) || sample.Timestamp.Before(sampleRetrievalRequest.To)

					if doesDataSourceIdMatch && doesFromTimeMatch && doesToTimeMatch {
						eligibleSamples = append(eligibleSamples, sample)
					}
				}

				sampleRetrievalRequest.ResponseChan <- eligibleSamples
			}
		}
	}()

	return r
}

type PersistentDataStoreReporter struct {
	dataStore         DataStore
	buffer            []*Sample
	flushTicker       *time.Ticker
	sampleChan        chan *Sample
	sampleRequestChan chan SampleRetrievalRequest
}

func (r *PersistentDataStoreReporter) OnSample(sample *Sample) {
	r.sampleChan <- sample
}

func (r *PersistentDataStoreReporter) GetSamples(dataSourceId string, from time.Time, to time.Time) []*Sample {
	responseChan := make(chan []*Sample)
	sampleFilterInfo := SampleRetrievalRequest{DataSourceId: dataSourceId, From: from, To: to, ResponseChan: responseChan}
	r.sampleRequestChan <- sampleFilterInfo
	return <-responseChan
}

func (r *PersistentDataStoreReporter) Prepare() error {
	return r.dataStore.Prepare()
}

func (r *PersistentDataStoreReporter) ShutDown() error {
	// TODO: stop ticker, flush remaining samples, and afterwards shutdown the data store
	return r.dataStore.ShutDown()
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type SampleRetrievalRequest struct {
	DataSourceId string
	From         time.Time
	To           time.Time
	ResponseChan chan []*Sample
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewKasperbrettReportingEngine() *KasperbrettReportingEngine {
	re := &KasperbrettReportingEngine{
		reporterRegistrationChan: make(chan Reporter),
		sampleChan:               make(chan *Sample),
		shutDownChan:             make(chan chan error),
	}

	go func() {
		for {
			select {
			case reporter := <-re.reporterRegistrationChan:
				re.reporters = append(re.reporters, reporter) // it is valid to append data to nil slices
				reporter.Prepare()                            // TODO: handle error that might happen during preparation
			case sample := <-re.sampleChan:
				for _, reporter := range re.reporters {
					go reporter.OnSample(sample)
				}
			case responseChan := <-re.shutDownChan:
				var overallErr error = nil
				for _, reporter := range re.reporters {
					err := reporter.ShutDown()
					if err != nil && overallErr == nil {
						overallErr = err
					}
				}
				responseChan <- overallErr
			}
		}
	}()

	return re
}

type KasperbrettReportingEngine struct {
	reporters                []Reporter
	reporterRegistrationChan chan Reporter
	sampleChan               chan *Sample
	shutDownChan             chan chan error
}

func (re *KasperbrettReportingEngine) Register(reporters ...Reporter) {
	for _, reporter := range reporters {
		re.reporterRegistrationChan <- reporter
	}
}

func (re *KasperbrettReportingEngine) Distribute(sample *Sample) {
	re.sampleChan <- sample
}

func (re *KasperbrettReportingEngine) ShutDown() error {
	responseChan := make(chan error)
	re.shutDownChan <- responseChan
	return <-responseChan
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

/* ***** ███████╗ ██████╗██╗  ██╗███████╗██████╗ ██╗   ██╗██╗     ██╗███╗   ██╗ ██████╗  ***** */
/* ***** ██╔════╝██╔════╝██║  ██║██╔════╝██╔══██╗██║   ██║██║     ██║████╗  ██║██╔════╝  ***** */
/* ***** ███████╗██║     ███████║█████╗  ██║  ██║██║   ██║██║     ██║██╔██╗ ██║██║  ███╗ ***** */
/* ***** ╚════██║██║     ██╔══██║██╔══╝  ██║  ██║██║   ██║██║     ██║██║╚██╗██║██║   ██║ ***** */
/* ***** ███████║╚██████╗██║  ██║███████╗██████╔╝╚██████╔╝███████╗██║██║ ╚████║╚██████╔╝ ***** */
/* ***** ╚══════╝ ╚═════╝╚═╝  ╚═╝╚══════╝╚═════╝  ╚═════╝ ╚══════╝╚═╝╚═╝  ╚═══╝ ╚═════╝  ***** */
/* ***** Banner comment generated by http://patorjk.com/software/taag ***** ***** ****** ***** */

// TODO: Refactor this section to a separate package!
// TODO: Add test cases

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type SchedulerJob struct {
	ticker *time.Ticker
	t      tomb.Tomb
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type Scheduler interface {
	Schedule(dataSourceId string, interval time.Duration, jobFn func(reportingEngine ReportingEngine)) (chan string, chan error)
	GetAll() (chan map[string]*SchedulerJob, chan error)
	Cancel(dataSourceId string) (chan bool, chan error)
	ShutDown() (chan bool, chan error)
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type SchedulerRequest interface {
	Run(registry map[string]*SchedulerJob, reportingEngine ReportingEngine)
	ErrorChan() chan error
	IsShutDownRoutine() bool
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewAbstractSchedulerRequest() AbstractSchedulerRequest {
	return AbstractSchedulerRequest{errorChan: make(chan error, 1)}
}

type AbstractSchedulerRequest struct {
	errorChan chan error
}

func (req AbstractSchedulerRequest) ErrorChan() chan error {
	return req.errorChan
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type AddJobRequest struct {
	AbstractSchedulerRequest
	responseChan chan string
	interval     time.Duration
	jobFn        func(reportingEngine ReportingEngine)
	dataSourceId string
}

func (req AddJobRequest) Run(registry map[string]*SchedulerJob, reportingEngine ReportingEngine) {
	job := &SchedulerJob{ticker: time.NewTicker(req.interval)}
	registry[req.dataSourceId] = job

	job.t.Go(func() error {
		for {
			select {
			case <-job.ticker.C:
				req.jobFn(reportingEngine)
			case <-job.t.Dying():
				return nil
			}
		}
	})

	req.responseChan <- req.dataSourceId
}

func (req AddJobRequest) IsShutDownRoutine() bool {
	return false
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type GetAllJobsRequest struct {
	AbstractSchedulerRequest
	responseChan chan map[string]*SchedulerJob
}

func (req GetAllJobsRequest) Run(registry map[string]*SchedulerJob, reportingEngine ReportingEngine) {
	registrySnapshot := make(map[string]*SchedulerJob, len(registry))

	for jobId, job := range registry {
		jobCopy := *job
		registrySnapshot[jobId] = &jobCopy
	}

	req.responseChan <- registrySnapshot
}

func (req GetAllJobsRequest) IsShutDownRoutine() bool {
	return false
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type RemoveJobRequest struct {
	AbstractSchedulerRequest
	responseChan chan bool
	jobId        string
}

func (req RemoveJobRequest) Run(registry map[string]*SchedulerJob, reportingEngine ReportingEngine) {
	job, ok := registry[req.jobId]
	var err error = nil
	if !ok {
		err = errors.New("Job not available: " + req.jobId)
	} else {
		job.ticker.Stop()
		delete(registry, req.jobId)
		job.t.Kill(nil)
		err = job.t.Wait()
	}
	req.errorChan <- err
	req.responseChan <- true
}

func (req RemoveJobRequest) IsShutDownRoutine() bool {
	return false
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

type RemoveAllJobsRequest struct {
	AbstractSchedulerRequest
	responseChan chan bool
}

func (req RemoveAllJobsRequest) Run(registry map[string]*SchedulerJob, reportingEngine ReportingEngine) {
	var overallErr error = nil
	for jobId, job := range registry {
		// TODO: remove redundant code
		job.ticker.Stop()
		delete(registry, jobId)
		job.t.Kill(nil)
		err := job.t.Wait()

		if err != nil && overallErr == nil {
			overallErr = err
		}
	}
	req.errorChan <- overallErr
	req.responseChan <- true
}

func (req RemoveAllJobsRequest) IsShutDownRoutine() bool {
	return true
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */

func NewKasperbrettScheduler(reportingEngine ReportingEngine) *KasperbrettScheduler {
	ks := &KasperbrettScheduler{
		reportingEngine: reportingEngine,
		requestChan:     make(chan SchedulerRequest),
		registry:        make(map[string]*SchedulerJob),
		isShutDown:      false,
	}

	go func() {
		for {
			req := <-ks.requestChan

			if ks.isShutDown {
				errorChan := req.ErrorChan()
				errorChan <- errors.New("The KasperbrettScheduler has already been shut down.")
			} else {
				req.Run(ks.registry, ks.reportingEngine)
				ks.isShutDown = req.IsShutDownRoutine()
			}
		}
	}()

	return ks
}

type KasperbrettScheduler struct {
	reportingEngine ReportingEngine
	requestChan     chan SchedulerRequest
	registry        map[string]*SchedulerJob
	isShutDown      bool
}

func (ks *KasperbrettScheduler) Schedule(dataSourceId string, interval time.Duration, jobFn func(reportingEngine ReportingEngine)) (chan string, chan error) {
	// we need a buffered chan in case the caller is not interested in the response value (jobId)
	// an unbuffered chan would block our request processing goroutine forever
	responseChan := make(chan string, 1)
	abstractSchedulerRequest := NewAbstractSchedulerRequest()

	req := AddJobRequest{
		AbstractSchedulerRequest: abstractSchedulerRequest,
		responseChan:             responseChan,
		interval:                 interval,
		jobFn:                    jobFn,
		dataSourceId:             dataSourceId,
	}
	ks.requestChan <- req

	return responseChan, abstractSchedulerRequest.ErrorChan()
}

func (ks *KasperbrettScheduler) GetAll() (chan map[string]*SchedulerJob, chan error) {
	// reasoning for buffered chan: see explanation in Schedule() impl.
	responseChan := make(chan map[string]*SchedulerJob, 1)
	abstractSchedulerRequest := NewAbstractSchedulerRequest()

	req := GetAllJobsRequest{
		AbstractSchedulerRequest: abstractSchedulerRequest,
		responseChan:             responseChan,
	}
	ks.requestChan <- req

	return responseChan, abstractSchedulerRequest.ErrorChan()
}

func (ks *KasperbrettScheduler) Cancel(jobId string) (chan bool, chan error) {
	// reasoning for buffered chan: see explanation in Schedule() impl.
	responseChan := make(chan bool, 1)
	abstractSchedulerRequest := NewAbstractSchedulerRequest()

	req := RemoveJobRequest{
		AbstractSchedulerRequest: abstractSchedulerRequest,
		responseChan:             responseChan,
		jobId:                    jobId,
	}
	ks.requestChan <- req

	return responseChan, abstractSchedulerRequest.ErrorChan()
}

func (ks *KasperbrettScheduler) ShutDown() (chan bool, chan error) {
	// reasoning for buffered chan: see explanation in Schedule() impl.
	responseChan := make(chan bool, 1)
	abstractSchedulerRequest := NewAbstractSchedulerRequest()

	req := RemoveAllJobsRequest{
		AbstractSchedulerRequest: abstractSchedulerRequest,
		responseChan:             responseChan,
	}
	ks.requestChan <- req

	return responseChan, abstractSchedulerRequest.ErrorChan()
}

/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
/* ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** ***** */
