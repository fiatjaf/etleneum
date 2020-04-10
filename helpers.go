package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/fiatjaf/hashbow"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/itchyny/gojq"
	"github.com/yudai/gojsondiff"
)

var wordMatcher *regexp.Regexp = regexp.MustCompile(`\b\w+\b`)

type Result struct {
	Ok    bool        `json:"ok"`
	Value interface{} `json:"value"`
	Error string      `json:"error,omitempty"`
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(Result{
		Ok:    false,
		Error: message,
	})
}

func diffDeltaOneliner(prefix string, idelta gojsondiff.Delta) (lines []string) {
	key := prefix
	if key != "" {
		key += "."
	}

	switch pdelta := idelta.(type) {
	case gojsondiff.PreDelta:
		switch delta := pdelta.(type) {
		case *gojsondiff.Moved:
			key = key + delta.PrePosition().String()
			lines = append(lines, fmt.Sprintf("- %s", key))
		case *gojsondiff.Deleted:
			key = key + delta.PrePosition().String()
			lines = append(lines, fmt.Sprintf("- %s", key[:len(key)-1]))
		}
	}

	switch pdelta := idelta.(type) {
	case gojsondiff.PostDelta:
		switch delta := pdelta.(type) {
		case *gojsondiff.TextDiff:
			key = key + delta.PostPosition().String()
			lines = append(lines, fmt.Sprintf("= %s %v", key, delta.NewValue))
		case *gojsondiff.Modified:
			key = key + delta.PostPosition().String()
			value, _ := json.Marshal(delta.NewValue)
			lines = append(lines, fmt.Sprintf("= %s %s", key, value))
		case *gojsondiff.Added:
			key = key + delta.PostPosition().String()
			value, _ := json.Marshal(delta.Value)
			lines = append(lines, fmt.Sprintf("+ %s %s", key, value))
		case *gojsondiff.Object:
			key = key + delta.PostPosition().String()
			for _, nextdelta := range delta.Deltas {
				lines = append(lines, diffDeltaOneliner(key, nextdelta)...)
			}
		case *gojsondiff.Array:
			key = key + delta.PostPosition().String()
			for _, nextdelta := range delta.Deltas {
				lines = append(lines, diffDeltaOneliner(key, nextdelta)...)
			}
		case *gojsondiff.Moved:
			key = key + delta.PostPosition().String()
			value, _ := json.Marshal(delta.Value)
			lines = append(lines, fmt.Sprintf("+ %s %s", key, value))
			if delta.Delta != nil {
				if d, ok := delta.Delta.(gojsondiff.Delta); ok {
					lines = append(lines, diffDeltaOneliner(key, d)...)
				}
			}
		}
	}

	return
}

func runJQ(
	ctx context.Context,
	input []byte,
	filter string,
) (result interface{}, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*2)
	defer cancel()

	query, err := gojq.Parse(filter)
	if err != nil {
		return
	}

	var object map[string]interface{}
	err = json.Unmarshal(input, &object)
	if err != nil {
		return nil, err
	}

	iter := query.RunWithContext(ctx, object)
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if err, ok := v.(error); ok {
		return nil, err
	}
	return v, nil
}

func generateLnurlImage(contractId string, method string) (b64 string, err error) {
	// load existing image
	base, err := Asset("static/lnurlpayicon.png")
	if err != nil {
		return
	}
	img, err := png.Decode(bytes.NewBuffer(base))
	if err != nil {
		return
	}

	// load font to write
	fontbytes, err := Asset("static/Inconsolata-Bold.ttf")
	if err != nil {
		return
	}
	f, err := truetype.Parse(fontbytes)
	if err != nil {
		return
	}
	face := truetype.NewFace(f, &truetype.Options{Size: 20})

	// create new image with gg
	bounds := img.Bounds()
	dc := gg.NewContext(bounds.Max.X, bounds.Max.Y)
	dc.DrawImage(img, 0, 0)

	// apply filters
	// contract filter
	hexcolor := hashbow.Hashbow(contractId)
	r, _ := strconv.ParseInt(hexcolor[1:3], 16, 64)
	g, _ := strconv.ParseInt(hexcolor[3:5], 16, 64)
	b, _ := strconv.ParseInt(hexcolor[5:7], 16, 64)
	dc.SetRGBA255(int(r), int(g), int(b), 120)
	dc.MoveTo(0, float64(bounds.Max.Y)*0.63)
	dc.CubicTo(
		float64(bounds.Max.X)*0.33, float64(bounds.Max.Y)*0.9,
		float64(bounds.Max.X)*0.80, float64(bounds.Max.Y)*0.25,
		float64(bounds.Max.X), float64(bounds.Max.Y)*0.3,
	)
	dc.LineTo(float64(bounds.Max.X), 0)
	dc.LineTo(0, 0)
	dc.Fill()

	// method filter
	hexcolor = hashbow.Hashbow(method)
	r, _ = strconv.ParseInt(hexcolor[1:3], 16, 64)
	g, _ = strconv.ParseInt(hexcolor[3:5], 16, 64)
	b, _ = strconv.ParseInt(hexcolor[5:7], 16, 64)
	dc.SetRGBA255(int(r), int(g), int(b), 120)
	dc.MoveTo(0, float64(bounds.Max.Y)*0.63)
	dc.CubicTo(
		float64(bounds.Max.X)*0.33, float64(bounds.Max.Y)*0.9,
		float64(bounds.Max.X)*0.80, float64(bounds.Max.Y)*0.25,
		float64(bounds.Max.X), float64(bounds.Max.Y)*0.3,
	)
	dc.LineTo(float64(bounds.Max.X), float64(bounds.Max.Y))
	dc.LineTo(0, float64(bounds.Max.Y))
	dc.Fill()

	// write contract id and method
	dc.SetFontFace(face)
	dc.SetRGB255(255, 255, 255)
	w, _ := dc.MeasureString(contractId)
	dc.DrawString(contractId, float64(bounds.Max.X)-8-w, 19)
	dc.DrawString(method+"()", 8, float64(bounds.Max.Y-9))

	// encode to base64 png and return
	out := bytes.Buffer{}
	err = dc.EncodePNG(&out)
	if err != nil {
		return
	}

	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}
