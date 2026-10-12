package bcc

import "strings"

// countryPoint is where the Command Center map draws a node of that country:
// the capital or main hosting city, never a guess at the server's own address.
type countryPoint struct{ Lat, Lon float64 }

// countryPoints covers the countries BAFT nodes are usually hosted in. A node
// outside this list can still run; it only stays off the map.
var countryPoints = map[string]countryPoint{
	"AE": {25.20, 55.27}, "AM": {40.18, 44.51}, "AR": {-34.60, -58.38}, "AT": {48.21, 16.37},
	"AU": {-33.87, 151.21}, "AZ": {40.41, 49.87}, "BE": {50.85, 4.35}, "BG": {42.70, 23.32},
	"BH": {26.23, 50.59}, "BR": {-23.55, -46.63}, "CA": {43.65, -79.38}, "CH": {47.37, 8.54},
	"CL": {-33.45, -70.67}, "CN": {39.90, 116.40}, "CY": {35.17, 33.36}, "CZ": {50.08, 14.44},
	"DE": {50.11, 8.68}, "DK": {55.68, 12.57}, "EE": {59.44, 24.75}, "EG": {30.04, 31.24},
	"ES": {40.42, -3.70}, "FI": {60.17, 24.94}, "FR": {48.86, 2.35}, "GB": {51.51, -0.13},
	"GE": {41.72, 44.79}, "GR": {37.98, 23.73}, "HK": {22.32, 114.17}, "HR": {45.81, 15.98},
	"HU": {47.50, 19.04}, "ID": {-6.21, 106.85}, "IE": {53.35, -6.26}, "IL": {32.09, 34.78},
	"IN": {19.08, 72.88}, "IQ": {33.31, 44.36}, "IR": {35.69, 51.39}, "IS": {64.15, -21.94},
	"IT": {45.46, 9.19}, "JP": {35.68, 139.69}, "KR": {37.57, 126.98}, "KW": {29.38, 47.99},
	"KZ": {43.24, 76.89}, "LT": {54.69, 25.28}, "LU": {49.61, 6.13}, "LV": {56.95, 24.11},
	"MD": {47.01, 28.86}, "MX": {19.43, -99.13}, "MY": {3.14, 101.69}, "NL": {52.37, 4.90},
	"NO": {59.91, 10.75}, "NZ": {-36.85, 174.76}, "OM": {23.59, 58.41}, "PK": {24.86, 67.01},
	"PL": {52.23, 21.01}, "PT": {38.72, -9.14}, "QA": {25.29, 51.53}, "RO": {44.43, 26.10},
	"RS": {44.79, 20.45}, "RU": {55.76, 37.62}, "SA": {24.71, 46.68}, "SE": {59.33, 18.07},
	"SG": {1.35, 103.82}, "SI": {46.06, 14.51}, "SK": {48.15, 17.11}, "TH": {13.76, 100.50},
	"TR": {41.01, 28.98}, "TW": {25.03, 121.57}, "UA": {50.45, 30.52}, "US": {40.71, -74.01},
	"UZ": {41.30, 69.24}, "VN": {10.82, 106.63}, "ZA": {-26.20, 28.05},
}

// normalizeCountry returns the upper-case ISO 3166-1 alpha-2 code, "" to clear
// it, and false for a code the map does not know.
func normalizeCountry(code string) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return "", true
	}
	_, ok := countryPoints[code]
	return code, ok
}
