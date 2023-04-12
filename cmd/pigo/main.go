package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	pigo "proglove_pigo/core"
	"proglove_pigo/utils"

	"github.com/fogleman/gg"
	"golang.org/x/term"
)

const banner = `
┌─┐┬┌─┐┌─┐
├─┘││ ┬│ │
┴  ┴└─┘└─┘

Go (Golang) Face detection library.
    Version: %s

`

// pipeName is the file name that indicates stdin/stdout is being used.
const pipeName = "-"

const (
	// markerRectangle - use rectangle as face detection marker
	markerRectangle string = "rect"
	// markerCircle - use circle as face detection marker
	markerCircle string = "circle"
	// markerEllipse - use ellipse as face detection marker
	markerEllipse string = "ellipse"

	// message colors
	successColor = "\x1b[92m"
	errorColor   = "\x1b[31m"
	defaultColor = "\x1b[0m"

	perturb = 63
)

// Version indicates the current build version.
var Version string

var (
	dc        *gg.Context
	det       *faceDetector
	plc       *pigo.PuplocCascade
	flpcs     map[string][]*pigo.FlpCascade
	imgParams *pigo.ImageParams
)

var (
	eyeCascades   = [5]string{"lp46", "lp44", "lp42", "lp38", "lp312"}
	mouthCascades = [4]string{"lp93", "lp84", "lp82", "lp81"}
)

// faceDetector struct contains Pigo face detector general settings.
type faceDetector struct {
	cascadeFile  string
	destination  string
	puploc       string
	flploc       string
	angle        float64
	minSize      int
	maxSize      int
	shiftFactor  float64
	scaleFactor  float64
	iouThreshold float64
	markDetEyes  bool
}

// coord holds the detection coordinates
type coord struct {
	Row   int `json:"x,omitempty"`
	Col   int `json:"y,omitempty"`
	Scale int `json:"size,omitempty"`
}

// detection holds the detection points of the various detection types
type detection struct {
	EyePoints      []coord `json:"eyes,omitempty"`
	LandmarkPoints []coord `json:"landmark_points,omitempty"`
	FacePoints     coord   `json:"face,omitempty"`
}

func main() {
	var (
		// Flags
		source       = flag.String("in", pipeName, "Source image")
		destination  = flag.String("out", pipeName, "Destination image")
		cascadeFile  = flag.String("cf", "", "Cascade binary file")
		minSize      = flag.Int("min", 20, "Minimum size of face")
		maxSize      = flag.Int("max", 1000, "Maximum size of face")
		shiftFactor  = flag.Float64("shift", 0.15, "Shift detection window by percentage")
		scaleFactor  = flag.Float64("scale", 1.15, "Scale detection window by percentage")
		angle        = flag.Float64("angle", 0.0, "0.0 is 0 radians and 1.0 is 2*pi radians")
		iouThreshold = flag.Float64("iou", 0.15, "Intersection over union (IoU) threshold")
		marker       = flag.String("marker", "rect", "Detection marker: rect|circle|ellipse")
		puploc       = flag.String("plc", "", "Pupils/eyes localization cascade file")
		flploc       = flag.String("flpc", "", "Facial landmark points cascade directory")
		markEyes     = flag.Bool("mark", true, "Mark detected eyes")
		jsonf        = flag.String("json", "", "Output the detection points into a json file")
	)

	log.SetFlags(0)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, fmt.Sprintf(banner, Version))
		flag.PrintDefaults()
	}
	flag.Parse()

	if len(*source) == 0 || len(*cascadeFile) == 0 {
		log.Fatal("Usage: pigo -in input.jpg -out out.png -cf cascade/facefinder")
	}

	start := time.Now()

	// Progress indicator
	spinner := utils.NewSpinner("Detecting faces...", time.Millisecond*100)
	spinner.Start()

	det = &faceDetector{
		angle:        *angle,
		destination:  *destination,
		cascadeFile:  *cascadeFile,
		minSize:      *minSize,
		maxSize:      *maxSize,
		shiftFactor:  *shiftFactor,
		scaleFactor:  *scaleFactor,
		iouThreshold: *iouThreshold,
		puploc:       *puploc,
		flploc:       *flploc,
		markDetEyes:  *markEyes,
	}

	var dst io.Writer
	if det.destination != "empty" {
		if det.destination == pipeName {
			if term.IsTerminal(int(os.Stdout.Fd())) {
				log.Fatalln("`-` should be used with a pipe for stdout")
			}
			dst = os.Stdout
		} else {
			fileTypes := []string{".jpg", ".jpeg", ".png"}
			ext := filepath.Ext(strings.ToLower(det.destination))

			if !inSlice(ext, fileTypes) {
				log.Fatalf("Output file type not supported: %v", ext)
			}

			fn, err := os.OpenFile(det.destination, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				log.Fatalf("Unable to open output file: %v", err)
			}
			defer fn.Close()
			dst = fn
		}
	}

	faces, err := det.detectFaces(*source)
	if err != nil {
		spinner.StopMsg = fmt.Sprintf("Detecting faces... %s failed ✗%s\n", errorColor, defaultColor)
		spinner.Stop()
		log.Fatalf("Detection error: %s%v%s", errorColor, err, defaultColor)
	}

	dets, err := det.drawFaces(faces, *marker)
	if err != nil {
		log.Fatalf("Error creating the image output: %s", err)
	}

	if det.destination != "empty" {
		if err := det.encodeImage(dst); err != nil {
			log.Fatalf("Error encoding the output image: %v", err)
		}
	}

	var out io.Writer
	if *jsonf != "" {
		if *jsonf == pipeName {
			out = os.Stdout
		} else {
			f, err := os.Create(*jsonf)
			if err != nil {
				log.Printf("%s", err)
			}
			defer f.Close()
			if err != nil {
				spinner.StopMsg = fmt.Sprintf("Detecting faces... %s failed ✗%s\n", errorColor, defaultColor)
				spinner.Stop()
				log.Fatalf(fmt.Sprintf("%sCould not create the json file: %v%s", errorColor, err, defaultColor))
			}
			out = f
		}

	}
	spinner.StopMsg = fmt.Sprintf("Detecting faces... %s✔%s", successColor, defaultColor)
	spinner.Stop()

	if len(dets) > 0 {
		log.Printf("\n%s%d%s face(s) detected", successColor, len(dets), defaultColor)

		if *jsonf != "" && out == os.Stdout {
			log.Printf("\n%sThe detection coordinates of the found faces:%s", successColor, defaultColor)
		}

		if out != nil {
			if err := json.NewEncoder(out).Encode(dets); err != nil {
				log.Fatalf("Error encoding the json file: %s", err)
			}
		}
	} else {
		log.Printf("\n%sno detected faces!%s", errorColor, defaultColor)
	}

	log.Printf("\nExecution time: %s%.2fs%s\n", successColor, time.Since(start).Seconds(), defaultColor)
}

// detectFaces run the detection algorithm over the provided source image.
func (fd *faceDetector) detectFaces(source string) ([]pigo.Detection, error) {
	var srcFile io.Reader

	// Check if source path is a local image or URL.
	if utils.IsValidUrl(source) {
		src, err := utils.DownloadImage(source)
		if err != nil {
			return nil, err
		}
		// Close and remove the generated temporary file.
		defer src.Close()
		defer os.Remove(src.Name())

		img, err := os.Open(src.Name())
		if err != nil {
			return nil, err
		}
		srcFile = img
	} else {
		if source == pipeName {
			if term.IsTerminal(int(os.Stdin.Fd())) {
				log.Fatalln("`-` should be used with a pipe for stdin")
			}
			srcFile = os.Stdin
		} else {
			file, err := os.Open(source)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			srcFile = file
		}
	}

	src, err := pigo.DecodeImage(srcFile)
	if err != nil {
		return nil, err
	}

	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Max.X, src.Bounds().Max.Y

	dc = gg.NewContext(cols, rows)
	dc.DrawImage(src, 0, 0)

	imgParams = &pigo.ImageParams{
		Pixels: pixels,
		Rows:   rows,
		Cols:   cols,
		Dim:    cols,
	}

	cParams := pigo.CascadeParams{
		MinSize:     det.minSize,
		MaxSize:     det.maxSize,
		ShiftFactor: det.shiftFactor,
		ScaleFactor: det.scaleFactor,
		ImageParams: *imgParams,
	}

	cascadeFile, err := ioutil.ReadFile(det.cascadeFile)
	if err != nil {
		return nil, fmt.Errorf("error reading the facefinder cascade file")
	}

	contentType, err := utils.DetectFileContentType(det.cascadeFile)
	if err != nil {
		return nil, err
	}
	if contentType != "application/octet-stream" {
		return nil, fmt.Errorf("the provided cascade classifier is not valid")
	}

	p := pigo.NewPigo()
	// Unpack the binary file. This will return the number of cascade trees,
	// the tree depth, the threshold and the prediction from tree's leaf nodes.
	classifier, err := p.Unpack(cascadeFile)
	if err != nil {
		return nil, err
	}

	plcReader := func() (*pigo.PuplocCascade, error) {
		plc := pigo.NewPuplocCascade()
		cascade, err := ioutil.ReadFile(det.puploc)
		if err != nil {
			return nil, fmt.Errorf("error reading the puploc cascade file")
		}
		plc, err = plc.UnpackCascade(cascade)
		if err != nil {
			return nil, err
		}
		return plc, nil
	}

	if len(det.puploc) > 0 {
		plc, err = plcReader()
		if err != nil {
			return nil, err
		}
	}

	if len(det.flploc) > 0 {
		plc, err = plcReader()
		if err != nil {
			return nil, fmt.Errorf("the puploc cascade file is required: use the -plc flag")
		}
		flpcs, err = plc.ReadCascadeDir(det.flploc)
		if err != nil {
			return nil, fmt.Errorf("error reading the facial landmark points directory")
		}
	}

	// Run the classifier over the obtained leaf nodes and return the detection results.
	// The result contains quadruplets representing the row, column, scale and detection score.
	faces := classifier.RunCascade(cParams, det.angle)

	// Calculate the intersection over union (IoU) of two clusters.
	faces = classifier.ClusterDetections(faces, det.iouThreshold)

	return faces, nil
}

// drawFaces marks the detected faces with the marker type defined as parameter (rectangle|circle|ellipse).
func (fd *faceDetector) drawFaces(faces []pigo.Detection, marker string) ([]detection, error) {
	var qThresh float32 = 5.0

	var (
		detections     = make([]detection, 0, len(faces))
		eyesCoords     = make([]coord, 0, len(faces))
		landmarkCoords = make([]coord, 0, len(faces))
		// puploc         *pigo.Puploc
	)

	for _, face := range faces {
		if face.Q > qThresh {
			switch marker {
			case markerRectangle:
				dc.DrawRectangle(float64(face.Col-face.Scale/2),
					float64(face.Row-face.Scale/2),
					float64(face.Scale),
					float64(face.Scale),
				)
			case markerCircle:
				dc.DrawArc(
					float64(face.Col),
					float64(face.Row),
					float64(face.Scale/2),
					0,
					2*math.Pi,
				)
			case markerEllipse:
				dc.DrawEllipse(
					float64(face.Col),
					float64(face.Row),
					float64(face.Scale)/2,
					float64(face.Scale)/1.6,
				)
			}
			faceCoord := &coord{
				Col:   face.Row - face.Scale/2,
				Row:   face.Col - face.Scale/2,
				Scale: face.Scale,
			}

			dc.SetFillStyle(gg.NewSolidPattern(color.RGBA{R: 128, G: 128, B: 128, A: 255}))
			dc.Fill()

			detections = append(detections, detection{
				FacePoints:     *faceCoord,
				EyePoints:      eyesCoords,
				LandmarkPoints: landmarkCoords,
			})
		}
	}
	return detections, nil
}

func (fd *faceDetector) encodeImage(dst io.Writer) error {
	var err error
	img := dc.Image()

	switch dst := dst.(type) {
	case *os.File:
		ext := filepath.Ext(strings.ToLower(dst.Name()))
		switch ext {
		case "", ".jpg", ".jpeg":
			err = jpeg.Encode(dst, img, &jpeg.Options{Quality: 100})
		case ".png":
			err = png.Encode(dst, img)
		default:
			err = errors.New("unsupported image format")
		}
	default:
		err = jpeg.Encode(dst, img, &jpeg.Options{Quality: 100})
	}
	return err
}

// inSlice checks if the item exists in the slice.
func inSlice(item string, slice []string) bool {
	for _, it := range slice {
		if it == item {
			return true
		}
	}
	return false
}
