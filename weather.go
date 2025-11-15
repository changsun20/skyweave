package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

var openWeatherAPIKey string

func init() {
	openWeatherAPIKey = os.Getenv("OPENWEATHER_API_KEY")
	if openWeatherAPIKey == "" {
		// For development, allow empty key (will skip API calls)
		fmt.Println("Warning: OPENWEATHER_API_KEY not set")
	}
}

// GeocodingResult represents a geocoding API response
type GeocodingResult struct {
	Name    string            `json:"name"`
	Lat     float64           `json:"lat"`
	Lon     float64           `json:"lon"`
	Country string            `json:"country"`
	State   string            `json:"state,omitempty"`
	Local   map[string]string `json:"local_names,omitempty"`
}

// HistoricalWeatherResponse represents historical weather data from History API
type HistoricalWeatherResponse struct {
	Message string `json:"message"`
	Cod     string `json:"cod"`
	CityID  int    `json:"city_id"`
	Cnt     int    `json:"cnt"`
	List    []struct {
		Dt   int64 `json:"dt"`
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Pressure  int     `json:"pressure"`
			Humidity  int     `json:"humidity"`
			TempMin   float64 `json:"temp_min"`
			TempMax   float64 `json:"temp_max"`
		} `json:"main"`
		Wind struct {
			Speed float64 `json:"speed"`
			Deg   int     `json:"deg"`
		} `json:"wind"`
		Clouds struct {
			All int `json:"all"`
		} `json:"clouds"`
		Weather []struct {
			ID          int    `json:"id"`
			Main        string `json:"main"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Rain *struct {
			OneH float64 `json:"1h,omitempty"`
		} `json:"rain,omitempty"`
		Snow *struct {
			OneH float64 `json:"1h,omitempty"`
		} `json:"snow,omitempty"`
	} `json:"list"`
}

// ForecastResponse represents 16-day forecast data
type ForecastResponse struct {
	Cod  string `json:"cod"`
	Cnt  int    `json:"cnt"`
	List []struct {
		Dt   int64 `json:"dt"`
		Temp struct {
			Day   float64 `json:"day"`
			Min   float64 `json:"min"`
			Max   float64 `json:"max"`
			Night float64 `json:"night"`
			Eve   float64 `json:"eve"`
			Morn  float64 `json:"morn"`
		} `json:"temp"`
		FeelsLike struct {
			Day   float64 `json:"day"`
			Night float64 `json:"night"`
			Eve   float64 `json:"eve"`
			Morn  float64 `json:"morn"`
		} `json:"feels_like"`
		Pressure int     `json:"pressure"`
		Humidity int     `json:"humidity"`
		Speed    float64 `json:"speed"` // wind speed
		Deg      int     `json:"deg"`   // wind direction
		Clouds   int     `json:"clouds"`
		Weather  []struct {
			ID          int    `json:"id"`
			Main        string `json:"main"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Rain float64 `json:"rain,omitempty"`
		Snow float64 `json:"snow,omitempty"`
		Pop  float64 `json:"pop"` // probability of precipitation
	} `json:"list"`
}

// WeatherData unified structure for both historical and forecast
type WeatherData struct {
	Temp        float64
	FeelsLike   float64
	Pressure    int
	Humidity    int
	Clouds      int
	Visibility  int
	WindSpeed   float64
	WindDeg     int
	Condition   string
	Description string
	Rain        float64
	Snow        float64
}

// geocodeLocation converts location string to coordinates
// Supports: "city,country", "zipcode,country", or just "city"
func geocodeLocation(location string) (*GeocodingResult, error) {
	if openWeatherAPIKey == "" {
		return nil, fmt.Errorf("OpenWeather API key not configured")
	}

	// Try to detect if it's a zip code (contains only numbers and optionally country code)
	isZipCode := false
	for _, char := range location {
		if char >= '0' && char <= '9' {
			isZipCode = true
			break
		}
	}

	var apiURL string
	if isZipCode {
		// Use zip code API
		apiURL = fmt.Sprintf("http://api.openweathermap.org/geo/1.0/zip?zip=%s&appid=%s",
			url.QueryEscape(location), openWeatherAPIKey)
	} else {
		// Use direct geocoding API
		apiURL = fmt.Sprintf("http://api.openweathermap.org/geo/1.0/direct?q=%s&limit=1&appid=%s",
			url.QueryEscape(location), openWeatherAPIKey)
	}

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("geocoding API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read geocoding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geocoding API error: %s - %s", resp.Status, string(body))
	}

	if isZipCode {
		// Single result for zip code
		var result GeocodingResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse zip code response: %w", err)
		}
		return &result, nil
	} else {
		// Array result for direct geocoding
		var results []GeocodingResult
		if err := json.Unmarshal(body, &results); err != nil {
			return nil, fmt.Errorf("failed to parse geocoding response: %w", err)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("location not found")
		}
		return &results[0], nil
	}
}

// getHistoricalWeather fetches weather data for a specific date and location
func getHistoricalWeather(lat, lon float64, targetDate time.Time) (*WeatherData, error) {
	if openWeatherAPIKey == "" {
		return nil, fmt.Errorf("OpenWeather API key not configured")
	}

	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)

	// Check if date is within the last year
	if targetDate.Before(oneYearAgo) {
		return nil, fmt.Errorf("historical data only available for the past year (since %s)", oneYearAgo.Format("2006-01-02"))
	}

	// If date is in the future (up to 16 days), use forecast API
	if targetDate.After(now) {
		daysAhead := int(targetDate.Sub(now).Hours() / 24)
		if daysAhead > 16 {
			return nil, fmt.Errorf("forecast only available for up to 16 days ahead")
		}
		return getForecastWeather(lat, lon, daysAhead)
	}

	// Use History API for past dates
	// Set time to noon of target date
	startTime := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(24 * time.Hour)

	apiURL := fmt.Sprintf("https://history.openweathermap.org/data/2.5/history/city?lat=%f&lon=%f&type=hour&start=%d&end=%d&units=metric&appid=%s",
		lat, lon, startTime.Unix(), endTime.Unix(), openWeatherAPIKey)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("history API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read history response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("history API error: %s - %s", resp.Status, string(body))
	}

	var histData HistoricalWeatherResponse
	if err := json.Unmarshal(body, &histData); err != nil {
		return nil, fmt.Errorf("failed to parse history response: %w", err)
	}

	if len(histData.List) == 0 {
		return nil, fmt.Errorf("no historical data available for this date")
	}

	// Average the hourly data to get daily summary
	return aggregateHistoricalData(&histData), nil
}

// getForecastWeather fetches forecast data for future dates
func getForecastWeather(lat, lon float64, daysAhead int) (*WeatherData, error) {
	apiURL := fmt.Sprintf("https://api.openweathermap.org/data/2.5/forecast/daily?lat=%f&lon=%f&cnt=%d&units=metric&appid=%s",
		lat, lon, daysAhead+1, openWeatherAPIKey)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("forecast API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read forecast response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("forecast API error: %s - %s", resp.Status, string(body))
	}

	var forecastData ForecastResponse
	if err := json.Unmarshal(body, &forecastData); err != nil {
		return nil, fmt.Errorf("failed to parse forecast response: %w", err)
	}

	if len(forecastData.List) == 0 {
		return nil, fmt.Errorf("no forecast data available")
	}

	// Get the target day (last day in the list)
	targetDay := forecastData.List[len(forecastData.List)-1]

	return convertForecastToWeatherData(&targetDay), nil
}

// aggregateHistoricalData averages hourly data into daily summary
func aggregateHistoricalData(histData *HistoricalWeatherResponse) *WeatherData {
	if len(histData.List) == 0 {
		return &WeatherData{}
	}

	var totalTemp, totalFeels, totalWind float64
	var totalPressure, totalHumidity, totalClouds int
	var rain, snow float64
	condition := ""
	description := ""

	// Get most common weather condition
	if len(histData.List[len(histData.List)/2].Weather) > 0 {
		midpoint := histData.List[len(histData.List)/2]
		condition = midpoint.Weather[0].Main
		description = midpoint.Weather[0].Description
	}

	// Average the values
	for _, item := range histData.List {
		totalTemp += item.Main.Temp
		totalFeels += item.Main.FeelsLike
		totalPressure += item.Main.Pressure
		totalHumidity += item.Main.Humidity
		totalClouds += item.Clouds.All
		totalWind += item.Wind.Speed

		if item.Rain != nil {
			rain += item.Rain.OneH
		}
		if item.Snow != nil {
			snow += item.Snow.OneH
		}
	}

	count := float64(len(histData.List))
	return &WeatherData{
		Temp:        totalTemp / count,
		FeelsLike:   totalFeels / count,
		Pressure:    int(float64(totalPressure) / count),
		Humidity:    int(float64(totalHumidity) / count),
		Clouds:      int(float64(totalClouds) / count),
		Visibility:  10000, // default value
		WindSpeed:   totalWind / count,
		Condition:   condition,
		Description: description,
		Rain:        rain,
		Snow:        snow,
	}
}

// convertForecastToWeatherData converts forecast data to WeatherData
func convertForecastToWeatherData(forecast *struct {
	Dt   int64 `json:"dt"`
	Temp struct {
		Day   float64 `json:"day"`
		Min   float64 `json:"min"`
		Max   float64 `json:"max"`
		Night float64 `json:"night"`
		Eve   float64 `json:"eve"`
		Morn  float64 `json:"morn"`
	} `json:"temp"`
	FeelsLike struct {
		Day   float64 `json:"day"`
		Night float64 `json:"night"`
		Eve   float64 `json:"eve"`
		Morn  float64 `json:"morn"`
	} `json:"feels_like"`
	Pressure int     `json:"pressure"`
	Humidity int     `json:"humidity"`
	Speed    float64 `json:"speed"`
	Deg      int     `json:"deg"`
	Clouds   int     `json:"clouds"`
	Weather  []struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
	Rain float64 `json:"rain,omitempty"`
	Snow float64 `json:"snow,omitempty"`
	Pop  float64 `json:"pop"`
}) *WeatherData {
	condition := ""
	description := ""
	if len(forecast.Weather) > 0 {
		condition = forecast.Weather[0].Main
		description = forecast.Weather[0].Description
	}

	return &WeatherData{
		Temp:        forecast.Temp.Day,
		FeelsLike:   forecast.FeelsLike.Day,
		Pressure:    forecast.Pressure,
		Humidity:    forecast.Humidity,
		Clouds:      forecast.Clouds,
		Visibility:  10000, // default
		WindSpeed:   forecast.Speed,
		WindDeg:     forecast.Deg,
		Condition:   condition,
		Description: description,
		Rain:        forecast.Rain,
		Snow:        forecast.Snow,
	}
}

// generatePrompt creates an AI prompt for image editing based on weather data
func generatePrompt(weatherData *WeatherData, locationName string) string {
	// Extract weather condition
	condition := weatherData.Condition
	if condition == "" {
		condition = "clear"
	}
	description := weatherData.Description
	if description == "" {
		description = "clear sky"
	}

	// Temperature in Celsius
	temp := weatherData.Temp
	tempDesc := ""
	if temp < 0 {
		tempDesc = "freezing cold"
	} else if temp < 10 {
		tempDesc = "cold"
	} else if temp < 20 {
		tempDesc = "cool"
	} else if temp < 28 {
		tempDesc = "warm"
	} else {
		tempDesc = "hot"
	}

	// Cloud coverage
	cloudiness := ""
	if weatherData.Clouds < 20 {
		cloudiness = "clear skies"
	} else if weatherData.Clouds < 50 {
		cloudiness = "partly cloudy skies"
	} else if weatherData.Clouds < 80 {
		cloudiness = "mostly cloudy skies"
	} else {
		cloudiness = "overcast skies"
	}

	// Visibility
	visibilityDesc := ""
	if weatherData.Visibility < 1000 {
		visibilityDesc = "with very poor visibility"
	} else if weatherData.Visibility < 5000 {
		visibilityDesc = "with reduced visibility"
	}

	// Rain/Snow
	precipitation := ""
	if weatherData.Rain > 0 {
		if weatherData.Rain < 2.5 {
			precipitation = "light rain"
		} else if weatherData.Rain < 10 {
			precipitation = "moderate rain"
		} else {
			precipitation = "heavy rain"
		}
	} else if weatherData.Snow > 0 {
		if weatherData.Snow < 2.5 {
			precipitation = "light snow"
		} else if weatherData.Snow < 10 {
			precipitation = "moderate snow"
		} else {
			precipitation = "heavy snow"
		}
	}

	// Wind
	windDesc := ""
	if weatherData.WindSpeed > 10 {
		windDesc = "with strong winds"
	} else if weatherData.WindSpeed > 5 {
		windDesc = "with moderate winds"
	}

	// Build the prompt
	prompt := fmt.Sprintf(
		"Transform this landscape photo to accurately depict %s weather conditions. "+
			"The scene should show %s (%s) with %s and a temperature of %.1f°C (%s). ",
		locationName, condition, description, cloudiness, temp, tempDesc,
	)

	if precipitation != "" {
		prompt += fmt.Sprintf("Add %s falling in the scene. ", precipitation)
	}

	if visibilityDesc != "" {
		prompt += fmt.Sprintf("The atmosphere should appear %s. ", visibilityDesc)
	}

	if windDesc != "" {
		prompt += fmt.Sprintf("Show signs of wind %s such as swaying trees or grass. ", windDesc)
	}

	prompt += fmt.Sprintf(
		"The lighting should match the cloudiness level (clouds: %d%%). "+
			"Maintain the original composition and main subjects of the photo while "+
			"authentically applying these weather conditions. The result should look "+
			"natural and photorealistic.",
		weatherData.Clouds,
	)

	return prompt
}
