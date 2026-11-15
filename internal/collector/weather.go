package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/viaiz/eink-dashboard/internal/model"
)

// Weather collects current conditions + daily forecast from Open-Meteo.
type Weather struct {
	BaseURL  string
	Location string
	Lat, Lon float64
	Days     int
	Timezone string
}

func (w *Weather) Key() string    { return "weather" }
func (w *Weather) Source() string { return "Open-Meteo" }

type openMeteoResp struct {
	Current struct {
		Time        string   `json:"time"`
		Temperature *float64 `json:"temperature_2m"`
		Humidity    *float64 `json:"relative_humidity_2m"`
		FeelsLike   *float64 `json:"apparent_temperature"`
		Code        *int     `json:"weather_code"`
		Wind        *float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Daily struct {
		Time   []string   `json:"time"`
		Code   []int      `json:"weather_code"`
		Max    []float64  `json:"temperature_2m_max"`
		Min    []float64  `json:"temperature_2m_min"`
		Precip []*float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

func (w *Weather) URL() string {
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(w.Lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(w.Lon, 'f', 4, 64))
	q.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max")
	q.Set("timezone", w.Timezone)
	days := w.Days
	if days <= 0 {
		days = 4
	}
	q.Set("forecast_days", strconv.Itoa(days))
	return w.BaseURL + "?" + q.Encode()
}

func (w *Weather) Fetch(ctx context.Context) (Result, error) {
	body, err := getBody(ctx, w.URL(), "application/json")
	if err != nil {
		return Result{}, err
	}
	data, err := ParseOpenMeteo(body, w.Location, w.Timezone)
	if err != nil {
		return Result{}, &PermanentError{err}
	}
	return Result{Data: data, SourceTime: model.Ptr(data.Current.Time)}, nil
}

// ParseOpenMeteo converts an Open-Meteo response into WeatherData.
func ParseOpenMeteo(body []byte, location, tz string) (*model.WeatherData, error) {
	var r openMeteoResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	c := r.Current
	if c.Temperature == nil || c.Humidity == nil || c.Code == nil {
		return nil, fmt.Errorf("open-meteo: response missing current fields")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", c.Time, loc)
	if err != nil {
		return nil, fmt.Errorf("open-meteo: bad time %q", c.Time)
	}
	out := &model.WeatherData{
		Location: location,
		Current: model.WeatherCurrent{
			Time: t, Temperature: *c.Temperature, Humidity: *c.Humidity,
			FeelsLike: c.FeelsLike, WindKmh: c.Wind, Code: *c.Code,
			Condition: WMOText(*c.Code), Icon: WMOIcon(*c.Code),
		},
	}
	dl := r.Daily
	n := len(dl.Time)
	if len(dl.Code) < n || len(dl.Max) < n || len(dl.Min) < n {
		return nil, fmt.Errorf("open-meteo: inconsistent daily arrays")
	}
	for i := 0; i < n; i++ {
		day := model.WeatherDay{
			Date: dl.Time[i], Code: dl.Code[i], Condition: WMOText(dl.Code[i]), Icon: WMOIcon(dl.Code[i]),
			MaxC: dl.Max[i], MinC: dl.Min[i],
		}
		if i < len(dl.Precip) {
			day.PrecipProb = dl.Precip[i]
		}
		out.Daily = append(out.Daily, day)
	}
	return out, nil
}

// WMOText maps WMO weather interpretation codes to short English text.
func WMOText(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61:
		return "Light rain"
	case 63:
		return "Rain"
	case 65:
		return "Heavy rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75, 77:
		return "Snow"
	case 80, 81:
		return "Rain showers"
	case 82:
		return "Violent showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm, hail"
	}
	return fmt.Sprintf("WMO code %d", code)
}

// WMOIcon maps WMO codes to an icon name in the dashboard icon set.
func WMOIcon(code int) string {
	switch {
	case code == 0:
		return "sun"
	case code == 1 || code == 2:
		return "partly"
	case code == 3:
		return "cloud"
	case code == 45 || code == 48:
		return "fog"
	case code >= 51 && code <= 57:
		return "drizzle"
	case (code >= 61 && code <= 67) || (code >= 80 && code <= 82):
		return "rain"
	case (code >= 71 && code <= 77) || code == 85 || code == 86:
		return "snow"
	case code >= 95:
		return "storm"
	}
	return "cloud"
}
