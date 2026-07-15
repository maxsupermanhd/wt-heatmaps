package imagecolorsort

import (
	"cmp"
	"errors"
	"image"
	"maps"
	"math"
	"slices"
	"sync"
)

type ImageFetchFn[K cmp.Ordered] func(id K) (*image.RGBA, error)

type ImageColorSort[K cmp.Ordered] struct {
	values  map[K]int
	lock    sync.Mutex
	fetchFn ImageFetchFn[K]
}

func NewImageColorSort[K cmp.Ordered](imageFetchFn ImageFetchFn[K]) *ImageColorSort[K] {
	return &ImageColorSort[K]{
		values:  map[K]int{},
		fetchFn: imageFetchFn,
	}
}

func (ics *ImageColorSort[K]) GetValues() map[K]int {
	ics.lock.Lock()
	defer ics.lock.Unlock()
	return maps.Clone(ics.values)
}

func (ics *ImageColorSort[K]) colorNOLOCK(id K) (int, error) {
	ret, ok := ics.values[id]
	if ok {
		return ret, nil
	}
	im, err := ics.fetchFn(id)
	if err != nil {
		return 0, err
	}
	ret = score(im)
	ics.values[id] = ret
	return ret, nil
}

// slopbegin: image color score

// score returns an integer score (0-360) based on the image's average hue.
// Colors gradually shift through the spectrum: reds → yellows → greens → cyans → blues → magentas → reds
func score(img *image.RGBA) int {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width == 0 || height == 0 {
		return 0
	}

	var sumR, sumG, sumB uint64
	pixelCount := width * height
	stride := img.Stride

	// Directly index Pix array for efficiency
	for y := 0; y < height; y++ {
		offset := y * stride
		for x := 0; x < width; x++ {
			idx := offset + x*4 // RGBA: 4 bytes per pixel
			sumR += uint64(img.Pix[idx])
			sumG += uint64(img.Pix[idx+1])
			sumB += uint64(img.Pix[idx+2])
			// Skip alpha channel
		}
	}

	// Calculate average color
	r := float64(sumR) / float64(pixelCount) / 255.0
	g := float64(sumG) / float64(pixelCount) / 255.0
	b := float64(sumB) / float64(pixelCount) / 255.0

	// Convert RGB to HSL hue (0-360 degrees)
	hue := rgbToHue(r, g, b)
	return int(hue)
}

func rgbToHue(r, g, b float64) float64 {
	max := math.Max(math.Max(r, g), b)
	min := math.Min(math.Min(r, g), b)

	if max == min {
		return 0 // Achromatic (whites, grays, blacks)
	}

	d := max - min
	var h float64

	switch max {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default: // b
		h = (r-g)/d + 4
	}

	return h * 60 // Convert to degrees (0-360)
}

// slopend: image color score

func (ics *ImageColorSort[K]) Sort(ids []K) error {
	ics.lock.Lock()
	defer ics.lock.Unlock()
	errs := []error{}
	slices.SortFunc(ids, func(a, b K) int {
		av, err := ics.colorNOLOCK(a)
		if err != nil {
			errs = append(errs, err)
		}
		bv, err := ics.colorNOLOCK(b)
		if err != nil {
			errs = append(errs, err)
		}
		if av == bv {
			return cmp.Compare(a, b)
		}
		return av - bv
	})
	return errors.Join(errs...)
}
