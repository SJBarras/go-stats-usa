package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type City struct {
	State string
	Name  string
	Lat   float64
	Lon   float64
}

type Current struct {
	Time          string  `json:"time"`
	Temperature   float64 `json:"temperature_2m"`
	WindSpeed     float64 `json:"wind_speed_10m"`
	WindDirection float64 `json:"wind_direction_10m"`
	WeatherCode   int     `json:"weather_code"`
}

type OMResponse struct {
	Latitude     float64           `json:"latitude"`
	Longitude    float64           `json:"longitude"`
	Timezone     string            `json:"timezone"`
	Current      Current           `json:"current"`
	CurrentUnits map[string]string `json:"current_units"`
}

type Result struct {
	City City
	Cur  Current
	Err  error
}

func main() {
	concurrency := flag.Int("concurrency", 10, "max number of simultaneous requests")
	singleThread := flag.Bool("single-thread", false, "run with a single OS thread (GOMAXPROCS=1) but still use goroutines")
	timeout := flag.Duration("timeout", 20*time.Second, "overall timeout for the run")
	fahrenheit := flag.Bool("f", true, "use Fahrenheit (otherwise Celsius)")
	mph := flag.Bool("mph", true, "use mph for wind speed")
	flag.Parse()

	if *singleThread {
		runtime.GOMAXPROCS(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := &http.Client{Timeout: 8 * time.Second}

	capitals := usStateCapitals()

	// semaphore to limit in-flight requests
	sem := make(chan struct{}, *concurrency)
	wg := sync.WaitGroup{}
	results := make(chan Result, len(capitals))

	for _, city := range capitals {
		wg.Add(1)
		go func(city City) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cur, err := fetchCurrent(ctx, client, city, *fahrenheit, *mph)
			results <- Result{City: city, Cur: cur, Err: err}
		}(city)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var out []Result
	for r := range results {
		out = append(out, r)
	}

	// sort results by state
	sort.Slice(out, func(i, j int) bool { return out[i].City.State < out[j].City.State })

	// print a simple table
	fmt.Printf("%s\n", strings.Repeat("-", 86))
	fmt.Printf("%-15s | %-18s | %9s | %7s | %3s | %s\n", "STATE", "CAPITAL", "TEMP", "WIND", "DIR", "AT")
	fmt.Printf("%s\n", strings.Repeat("-", 86))
	for _, r := range out {
		if r.Err != nil {
			fmt.Printf("%-15s | %-18s | %9s | %7s | %3s | %v\n", r.City.State, r.City.Name, "ERR", "-", "-", r.Err)
			continue
		}
		dir := windDir(r.Cur.WindDirection)
		fmt.Printf("%-15s | %-18s | %6.1f° | %5.1f | %3s | %s\n",
			r.City.State, r.City.Name, r.Cur.Temperature, r.Cur.WindSpeed, dir, r.Cur.Time)
	}
	fmt.Printf("%s\n", strings.Repeat("-", 86))
}

func fetchCurrent(ctx context.Context, client *http.Client, city City, fahrenheit, mph bool) (Current, error) {
	// Build URL for Open-Meteo current conditions
	unitsTemp := "celsius"
	if fahrenheit {
		unitsTemp = "fahrenheit"
	}
	unitsWind := "kmh"
	if mph {
		unitsWind = "mph"
	}

	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,weather_code,wind_speed_10m,wind_direction_10m&temperature_unit=%s&wind_speed_unit=%s",
		city.Lat, city.Lon, unitsTemp, unitsWind)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Current{}, err
	}
	req.Header.Set("User-Agent", "go-capitals-weather/1.0 (+https://example.local)")

	resp, err := client.Do(req)
	if err != nil {
		return Current{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Current{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var om OMResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&om); err != nil {
		return Current{}, err
	}
	// basic validation: ensure current has a timestamp
	if om.Current.Time == "" {
		return Current{}, errors.New("missing current conditions in response")
	}
	return om.Current, nil
}

func windDir(deg float64) string {
	dirs := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	idx := int((deg/22.5)+0.5) % 16
	return dirs[idx]
}

func usStateCapitals() []City {
	// Lat/Lon roughly for downtown/statehouse; good enough for demo purposes.
	return []City{
		{"Alabama", "Montgomery", 32.377716, -86.300568},
		{"Alaska", "Juneau", 58.301598, -134.420212},
		{"Arizona", "Phoenix", 33.448143, -112.096962},
		{"Arkansas", "Little Rock", 34.746613, -92.288986},
		{"California", "Sacramento", 38.576668, -121.493629},
		{"Colorado", "Denver", 39.739227, -104.984856},
		{"Connecticut", "Hartford", 41.764046, -72.682198},
		{"Delaware", "Dover", 39.157307, -75.519722},
		{"Florida", "Tallahassee", 30.438118, -84.281296},
		{"Georgia", "Atlanta", 33.748997, -84.387985},
		{"Hawaii", "Honolulu", 21.304850, -157.857758},
		{"Idaho", "Boise", 43.615021, -116.202316},
		{"Illinois", "Springfield", 39.798363, -89.654961},
		{"Indiana", "Indianapolis", 39.768402, -86.158066},
		{"Iowa", "Des Moines", 41.591087, -93.603729},
		{"Kansas", "Topeka", 39.047345, -95.675157},
		{"Kentucky", "Frankfort", 38.186722, -84.875374},
		{"Louisiana", "Baton Rouge", 30.457069, -91.187393},
		{"Maine", "Augusta", 44.307167, -69.781693},
		{"Maryland", "Annapolis", 38.978764, -76.490936},
		{"Massachusetts", "Boston", 42.358162, -71.063698},
		{"Michigan", "Lansing", 42.733635, -84.555328},
		{"Minnesota", "Saint Paul", 44.955097, -93.102211},
		{"Mississippi", "Jackson", 32.303848, -90.182106},
		{"Missouri", "Jefferson City", 38.579201, -92.172935},
		{"Montana", "Helena", 46.585709, -112.018417},
		{"Nebraska", "Lincoln", 40.808075, -96.699654},
		{"Nevada", "Carson City", 39.163914, -119.766121},
		{"New Hampshire", "Concord", 43.206898, -71.537994},
		{"New Jersey", "Trenton", 40.220596, -74.769913},
		{"New Mexico", "Santa Fe", 35.682240, -105.939728},
		{"New York", "Albany", 42.652843, -73.757874},
		{"North Carolina", "Raleigh", 35.780430, -78.639099},
		{"North Dakota", "Bismarck", 46.820850, -100.783318},
		{"Ohio", "Columbus", 39.961346, -82.999069},
		{"Oklahoma", "Oklahoma City", 35.492207, -97.503342},
		{"Oregon", "Salem", 44.938461, -123.030403},
		{"Pennsylvania", "Harrisburg", 40.264378, -76.883598},
		{"Rhode Island", "Providence", 41.830914, -71.414963},
		{"South Carolina", "Columbia", 34.000343, -81.033211},
		{"South Dakota", "Pierre", 44.367031, -100.346405},
		{"Tennessee", "Nashville", 36.165810, -86.784241},
		{"Texas", "Austin", 30.274670, -97.740349},
		{"Utah", "Salt Lake City", 40.777477, -111.888237},
		{"Vermont", "Montpelier", 44.262436, -72.580536},
		{"Virginia", "Richmond", 37.538857, -77.433640},
		{"Washington", "Olympia", 47.035805, -122.905014},
		{"West Virginia", "Charleston", 38.336246, -81.612328},
		{"Wisconsin", "Madison", 43.074684, -89.384445},
		{"Wyoming", "Cheyenne", 41.140259, -104.820236},
	}
}
