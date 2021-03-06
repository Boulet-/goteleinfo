package teleinfo

import (
	"bytes"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
)

// Frame holds a single Teleinfo frame.
type Frame interface {
	// Type returns the type of frame (see `OPTARIF` field)
	Type() string
	// GetStringField returns the value of a field as a string.
	GetStringField(string) (string, bool)
	// GetUIntField returns type value of a field as an unsigned integer.
	GetUIntField(string) (uint, bool)
}

type frame map[string]string

func (f frame) Type() string {
	return f["OPTARIF"]
}

func (f frame) GetStringField(name string) (string, bool) {
	v, ok := f[name]
	return v, ok
}

func (f frame) GetUIntField(name string) (uint, bool) {
	s, ok := f[name]
	if !ok {
		return 0, ok
	}
	num, err := strconv.ParseUint(s, 10, 32)
	ok = err == nil
	return uint(num), ok
}

// NOTES:
// * used by decodeFrame()
// * []byte cannot be const :(
var fieldSeparator = []byte("\r\n")
var eltSeparator = []byte(" ")

func decodeFrame(rawFrame []byte) (Frame, error) {
	const (
		checksumLength = 1
	)

	strFrame := bytes.Trim(rawFrame, "\r\n")

	fields := bytes.Split(strFrame, fieldSeparator)
	info := frame{}
	for _, field := range fields {
		elts := bytes.SplitN(field, eltSeparator, 3)

		if len(elts) != 3 {
			incrementErrorCounter(frameDecodeErrorCounter, "invalid_field")
			return nil, fmt.Errorf("error decoding frame, invalid number of elements for data (data: '%s')", field)
		}
		name, value, trail := elts[0], elts[1], elts[2]

		if len(trail) != checksumLength {
			incrementErrorCounter(frameDecodeErrorCounter, "invalid_checksum_length")
			return nil, fmt.Errorf("error decoding frame, invalid checksum length (actual: %d, expected: %d)", len(trail), checksumLength)
		}
		readChecksum := byte(trail[0])
		expectedChecksum := computeChecksum(name, value)
		if readChecksum != expectedChecksum {
			incrementErrorCounter(frameDecodeErrorCounter, "checksum_error")
			return nil, fmt.Errorf("error decoding frame, invalid checksum (field: '%s', value: '%s', read: '%c', expected: '%c'", name, value, readChecksum, expectedChecksum)
		}
		nameString := string(name)
		valueString := string(value)
		valueFloat := 0.0

		switch nameString {
		case "ADCO", "PPOT":
			// We don't collect theses metrics
		case "HCHC":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoHeureCreuseGauge.Set(valueFloat)
		case "HCHP":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoHeurePleinesGauge.Set(valueFloat)
		case "HHPHC":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoProgrammationHeuresPleinesHeuresCreusesGauge.With(prometheus.Labels{"programme": valueString}).Set(1)
		case "IINST1", "IINST2", "IINST3":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			phase := string(nameString[len(nameString)-1])
			teleinfoIntensiteInstantaneeGauge.With(prometheus.Labels{"phase": phase}).Set(valueFloat)
		case "IMAX1", "IMAX2", "IMAX3":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			phase := string(name[len(name)-1])
			teleinfoIntensiteMaximaleGauge.With(prometheus.Labels{"phase": phase}).Set(valueFloat)
		case "ISOUSC":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoIntensiteSouscriteGauge.Set(valueFloat)
		case "MOTDETAT":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoModeEtatCompteurGauge.Set(valueFloat)
		case "OPTARIF":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoOptionTarifaireChoisieGauge.With(prometheus.Labels{"tarif": valueString}).Set(1)
		case "PAPP":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoPuissanceApparenteTriphaseGauge.Set(valueFloat)
		case "PMAX":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoPuissanceMaximaleTriphaseGauge.Set(valueFloat)
		case "PTEC":
			valueFloat, _ = strconv.ParseFloat(valueString, 64)
			teleinfoPeriodeTarifaireEnCoursGauge.With(prometheus.Labels{"tarif": valueString}).Set(1)
		default:
			incrementErrorCounter(frameDecodeErrorCounter, "unknown_field")
			return nil, fmt.Errorf("Error creating metric, Unknown field (field: '%s', value: '%s')", name, value)
		}
		info[nameString] = valueString
	}
	frameDecodedCounter.Inc()
	return info, nil
}

func sum(a []byte) (res byte) {
	res = 0
	for _, c := range a {
		res += c
	}
	return
}

func computeChecksum(name []byte, value []byte) byte {
	// NOTE: 0x20 == ASCII space char
	checksum := sum(name) + byte(0x20) + sum(value)

	// Map to a single char E [0x20;0x7F]
	checksum = (checksum & 0x3F) + 0x20
	return checksum
}
