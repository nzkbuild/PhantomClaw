package market

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

// COTData holds Commitments of Traders report data for a commodity/currency.
type COTData struct {
	Symbol         string `json:"symbol"`
	CommLong       int64  `json:"commercial_long"`
	CommShort      int64  `json:"commercial_short"`
	CommNet        int64  `json:"commercial_net"`
	LargeLong      int64  `json:"large_spec_long"`
	LargeShort     int64  `json:"large_spec_short"`
	LargeNet       int64  `json:"large_spec_net"`
	SmallLong      int64  `json:"small_spec_long"`
	SmallShort     int64  `json:"small_spec_short"`
	SmallNet       int64  `json:"small_spec_net"`
	ReportDate     string `json:"report_date"`
	NetPositioning string `json:"net_positioning"` // "bullish" | "bearish" | "neutral"
}

// COTFetcher retrieves CFTC Commitments of Traders data.
type COTFetcher struct {
	db     *memory.DB
	client *http.Client
}

// NewCOTFetcher creates a COT data fetcher with cache support.
func NewCOTFetcher(db *memory.DB) *COTFetcher {
	return &COTFetcher{
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// symbolToCFTC maps forex symbols to CFTC contract market names.
var symbolToCFTC = map[string]string{
	"EURUSD": "EURO FX",
	"GBPUSD": "BRITISH POUND",
	"USDJPY": "JAPANESE YEN",
	"AUDUSD": "AUSTRALIAN DOLLAR",
	"NZDUSD": "NEW ZEALAND DOLLAR",
	"USDCAD": "CANADIAN DOLLAR",
	"USDCHF": "SWISS FRANC",
	"XAUUSD": "GOLD",
}

// FetchCOT retrieves the latest COT data for a symbol.
// COT reports are weekly (Tuesday data, released Friday) — cached for 7 days.
func (cf *COTFetcher) FetchCOT(symbol string) (*COTData, error) {
	cacheKey := "cot_" + symbol
	cached, found, err := cf.db.GetCache(cacheKey)
	if err == nil && found {
		if parsed, parseErr := parseCOTCache(cached); parseErr == nil {
			return parsed, nil
		}
	}

	contractName, ok := symbolToCFTC[symbol]
	if !ok {
		return nil, fmt.Errorf("cot: no CFTC mapping for %s", symbol)
	}

	// CFTC provides free CSV downloads
	// Using the current year's disaggregated futures report
	year := time.Now().Year()
	url := fmt.Sprintf("https://www.cftc.gov/dea/newcot/deacom%d.txt", year)

	resp, err := cf.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("cot: fetch error: %w", err)
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1 // Variable fields

	var latestRow []string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(record) > 0 && strings.Contains(strings.ToUpper(record[0]), contractName) {
			latestRow = record // Keep overwriting — last match = most recent
		}
	}

	if latestRow == nil {
		return nil, fmt.Errorf("cot: no data found for %s (%s)", symbol, contractName)
	}

	data := parseCOTRow(symbol, latestRow)

	// Cache for 7 days (weekly report)
	if payload, marshalErr := json.Marshal(data); marshalErr == nil {
		_ = cf.db.SetCache(cacheKey, string(payload), "cftc", time.Now().Add(7*24*time.Hour))
	}

	return data, nil
}

func parseCOTRow(symbol string, row []string) *COTData {
	data := &COTData{Symbol: symbol}

	// CFTC CSV columns vary but common structure:
	// Columns 7-9: Commercial Long/Short/Net
	// Columns 10-12: Large Spec Long/Short/Net
	if len(row) > 12 {
		data.CommLong, _ = strconv.ParseInt(strings.TrimSpace(row[7]), 10, 64)
		data.CommShort, _ = strconv.ParseInt(strings.TrimSpace(row[8]), 10, 64)
		data.CommNet = data.CommLong - data.CommShort
		data.LargeLong, _ = strconv.ParseInt(strings.TrimSpace(row[10]), 10, 64)
		data.LargeShort, _ = strconv.ParseInt(strings.TrimSpace(row[11]), 10, 64)
		data.LargeNet = data.LargeLong - data.LargeShort
	}

	// Net positioning based on commercial hedger bias
	switch {
	case data.CommNet > 0:
		data.NetPositioning = "bullish"
	case data.CommNet < 0:
		data.NetPositioning = "bearish"
	default:
		data.NetPositioning = "neutral"
	}

	return data
}

func parseCOTCache(cached string) (*COTData, error) {
	var data COTData
	if err := json.Unmarshal([]byte(cached), &data); err != nil {
		return nil, err
	}
	return &data, nil
}
