package imagecolorsort

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"maps"
	"math"
	"os"
	"slices"
	"sync"

	"github.com/maxsupermanhd/flexcorallib/fclcache"
)

type ImageFetchFn[K cmp.Ordered] func(id K) ([]byte, error)

type ImageColorSort[K cmp.Ordered] struct {
	values  map[K]int
	lock    sync.Mutex
	fetchFn ImageFetchFn[K]
	// key is the hex SHA-256 of the image file, value is the sort value
	byHash *fclcache.SavingMapCache[string, int]
}

func NewImageColorSort[K cmp.Ordered](cacheFilePath string, imageFetchFn ImageFetchFn[K]) (*ImageColorSort[K], error) {
	byHash, err := fclcache.NewSavingMapCache[string, int](cacheFilePath, nil)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(cacheFilePath)
	if err == nil {
		err = json.Unmarshal(b, &byHash.Cache)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", cacheFilePath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &ImageColorSort[K]{
		values:  map[K]int{},
		fetchFn: imageFetchFn,
		byHash:  byHash,
	}, nil
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
	ret, err := ics.colorFromFetch(id)
	ics.values[id] = ret
	if err != nil {
		return ret, fmt.Errorf("color of %v: %w", id, err)
	}
	return ret, nil
}

func (ics *ImageColorSort[K]) colorFromFetch(id K) (int, error) {
	b, err := ics.fetchFn(id)
	if err != nil {
		return 0, err
	}
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	ret, ok := ics.byHash.Cache[h]
	if ok {
		return ret, nil
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	imRGBA := image.NewRGBA(im.Bounds())
	draw.Draw(imRGBA, imRGBA.Rect, im, im.Bounds().Min, draw.Src)
	ret = score(imRGBA)
	ics.byHash.Cache[h] = ret
	return ret, ics.byHash.SaveNOLOCK()
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

	// Directly index Pix array for efficiency
	for i := 0; i < len(img.Pix)-3; i += 4 {
		sumR += uint64(img.Pix[i])
		sumG += uint64(img.Pix[i+1])
		sumB += uint64(img.Pix[i+2])
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

// Sort keeps an id with no sort value in ids at NeutralValue, and returns
// the errors of those ids.
func (ics *ImageColorSort[K]) Sort(ids []K) error {
	ics.lock.Lock()
	defer ics.lock.Unlock()
	errs := []error{}
	for _, id := range ids {
		_, err := ics.colorNOLOCK(id)
		if err != nil {
			errs = append(errs, err)
		}
	}
	slices.SortFunc(ids, func(a, b K) int {
		av, bv := ics.values[a], ics.values[b]
		if av == bv {
			return cmp.Compare(a, b)
		}
		return av - bv
	})
	return errors.Join(errs...)
}
