package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"net/http"
	"net/url"

	"github.com/chromedp/chromedp"
)

type DLIndex struct {
	Origin string `json:"origin"`
	Path   string `json:"path"`
}

type VideoTimePersistance struct {
	Path string  `json:"path"`
	Time float64 `json:"time"`
}

var shutdown bool = false
var DLIndexes []DLIndex
var TimePersistance []VideoTimePersistance
var InProgressDLs map[string]string = make(map[string]string)

func PingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	w.WriteHeader(200)
	w.Write([]byte("Pong!"))
	return
}

func LookForMatchInDLIndexes(originURL string) (bool, string) {
	for _, v := range DLIndexes {
		if v.Origin == originURL {
			return true, v.Path
		}
	}
	return false, ""
}

func updateSavedDLIndexes() {
	encoded, err := json.Marshal(DLIndexes)
	if err != nil {
		// Failed to JSON encode data
		log.Println("Failed to encode DLIndexes as JSON, error:", err)
		return
	}

	err = os.WriteFile("dlindex.json", encoded, 0777)
	if err != nil {
		// Failed to save dlindex
		log.Println("Failed to write to dlindex.json, error:", err)
	}
}

func AddDLToIndex(originURL string, path string) {
	var data DLIndex

	data.Origin = originURL
	data.Path = path

	DLIndexes = append(DLIndexes, data)

	updateSavedDLIndexes()
}

func DoesLocalCopyExist(originURL string) string {
	u, err := url.Parse(originURL)
	if err != nil {
		log.Println("Failed to parse URL:", originURL, "error:", err)
		return originURL
	}

	originURL = "https://soap2day.mx" + u.Path

	doesExist, path := LookForMatchInDLIndexes(originURL)

	if !doesExist {
		return originURL
	}

	if _, err := os.Stat(path); err == nil {
		// The video already has been downloaded, use this file instead.
		return "USECACHESERVER/CachedVideo::" + path
	} else {
		return originURL
	}
}

func CachedVideoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")

	if r.Method != "POST" && r.Method != "OPTIONS" {
		w.WriteHeader(405)
		w.Write([]byte("Method not allowed (" + r.Method + ")"))
		return
	} else if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Write([]byte(""))
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("/CachedVideo :: Failed to read body, error:", err)
		w.WriteHeader(500)
		w.Write([]byte("Sorry, something went wrong"))
		return
	}

	path := string(data)

	http.ServeFile(w, r, path)
}

func LookForMatchInTimePersistance(Path string) (bool, string, int) {
	for i, v := range TimePersistance {
		if v.Path == Path {
			return true, v.Path, i
		}
	}
	return false, "", -1
}

func updateSavedTimePersistance() {
	encoded, err := json.Marshal(TimePersistance)
	if err != nil {
		// Failed to JSON encode data
		log.Println("Failed to encode TimePersistance as JSON, error:", err)
		return
	}

	err = os.WriteFile("videopersistance.json", encoded, 0777)
	if err != nil {
		// Failed to save videopersistance
		log.Println("Failed to write to videopersistance.json, error:", err)
	}
}

func UpdateVideoTimePersistance(Path string, Time float64) {
	ok, _, index := LookForMatchInTimePersistance(Path)
	if !ok {
		TimePersistance = append(TimePersistance, VideoTimePersistance{
			Path: Path,
			Time: Time,
		})
	} else {
		TimePersistance[index] = VideoTimePersistance{
			Path: Path,
			Time: Time,
		}
	}

	log.Println(TimePersistance)

	updateSavedTimePersistance()
}

func SetCurrentTimeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")

	if r.Method != "POST" && r.Method != "OPTIONS" {
		w.WriteHeader(405)
		w.Write([]byte("Method not allowed (" + r.Method + ")"))
		return
	} else if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Write([]byte(""))
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("/SetCurrentTime :: Failed to read body, error:", err)
		w.WriteHeader(500)
		w.Write([]byte("Sorry, something went wrong"))
		return
	}

	var d struct {
		VideoPath string  `json:"videoPath"`
		Time      float64 `json:"time"`
	}

	json.Unmarshal(data, &d)

	UpdateVideoTimePersistance(d.VideoPath, d.Time)

	w.WriteHeader(200)
	w.Write([]byte("OK"))
	return
}

func GetNewPlayer(VideoPath string) string {
	HTMLB, err := os.ReadFile("player/player.html")
	if err != nil {
		log.Println("Failed to load player, error:", err)
		return `
			<html>
				<head>
					<title>Soap2Day Mirror | Player</title>
				</head>
				<body>
					<h1>Failed to load player</h1>
				</body>
			</html>
		`
	}

	HTML := string(HTMLB)

	lastTime := GetLastPlaybackTime(VideoPath)

	VideoURL := VideoPath

	if _, err := os.Stat(VideoPath); err != nil {
		VideoURL = InProgressDLs[VideoPath]
	}

	HTML = strings.ReplaceAll(HTML, "VIDEO_PLAYER_VIDEO_PATH", VideoPath)
	HTML = strings.ReplaceAll(HTML, "VIDEO_PLAYER_URL", VideoURL)
	HTML = strings.ReplaceAll(HTML, "{VIDEO_START_POINT}", fmt.Sprint(lastTime))

	return HTML
}

func GetLastPlaybackTime(Path string) float64 {
	ok, _, index := LookForMatchInTimePersistance(Path)

	if !ok {
		return 0.0
	}

	return TimePersistance[index].Time
}

func GetVideoPlayerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")

	if r.Method != "POST" && r.Method != "OPTIONS" {
		w.WriteHeader(405)
		w.Write([]byte("Method not allowed (" + r.Method + ")"))
		return
	} else if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Write([]byte(""))
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("/GetPlayerHandler :: Failed to read body, error:", err)
		w.WriteHeader(500)
		w.Write([]byte("Sorry, something went wrong"))
		return
	}

	if !strings.HasPrefix(string(data), "media/") {
		data = []byte("media/" + string(data))
	}

	HTML := GetNewPlayer(string(data))
	w.WriteHeader(200)
	w.Write([]byte(HTML))
	return

	// if _, err := os.Stat(string(data)); err == nil {
	// 	HTML := GetNewPlayer(string(data))
	// 	w.WriteHeader(200)
	// 	w.Write([]byte(HTML))
	// 	return
	// }

	// log.Println(string(data))

	// w.WriteHeader(200)
	// w.Write([]byte(InProgressDLs[string(data)]))
	// return
}

func GetVideoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")

	if r.Method != "GET" && r.Method != "OPTIONS" {
		w.WriteHeader(405)
		w.Write([]byte("Method not allowed (" + r.Method + ")"))
		return
	} else if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Write([]byte(""))
		return
	}

	h, err := os.ReadFile("player/index.html")
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Failed to load player"))
		return
	}

	w.WriteHeader(200)
	w.Write([]byte(h))
	return
}

func GetPlayerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")

	if r.Method != "POST" && r.Method != "OPTIONS" {
		w.WriteHeader(405)
		w.Write([]byte("Method not allowed (" + r.Method + ")"))
		return
	} else if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Write([]byte(""))
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("/GetPlayerHandler :: Failed to read body, error:", err)
		w.WriteHeader(500)
		w.Write([]byte("Sorry, something went wrong"))
		return
	}

	var d struct {
		Page string `json:"page"`
	}

	json.Unmarshal(data, &d)

	newURL := DoesLocalCopyExist(d.Page)

	if strings.HasPrefix(newURL, "USECACHESERVER") {
		w.WriteHeader(200)
		w.Write([]byte(newURL))
		return
	}

	URL := extractPlayerURL(d.Page)

	// Download video
	URL = StartVideoDL(URL, d.Page, newURL == d.Page)

	if URL == "" {
		w.WriteHeader(500)
		w.Write([]byte("Sorry, something went wrong"))
		return
	}

	w.WriteHeader(200)
	w.Write([]byte(URL))
	return
}

func Round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}

func GetFormattedSize(size float64) string {
	var suffixes [5]string
	// size := sizeInMB * 1024 * 1024 // This is in bytes
	suffixes[0] = "B"
	suffixes[1] = "KB"
	suffixes[2] = "MB"
	suffixes[3] = "GB"
	suffixes[4] = "TB"

	base := math.Log(size) / math.Log(1024)
	getSize := Round(math.Pow(1024, base-math.Floor(base)), .5, 2)
	getSuffix := suffixes[int(math.Floor(base))]
	return fmt.Sprintf(strconv.FormatFloat(getSize, 'f', -1, 64) + " " + string(getSuffix))
}

func secondsToMinutes(inSeconds int64) string {
	if inSeconds == 0 {
		return "∞"
	}
	minutes := inSeconds / 60
	seconds := inSeconds % 60
	str := fmt.Sprintf("%dm %ds", minutes, seconds)
	if minutes < 0 || seconds < 0 {
		return "∞"
	}
	return str
}

func PrintDownloadPercent(VideoName string, done chan int64, path string, total int64) {

	var stop bool = false

	var startTime int64 = time.Now().UnixMilli()

	var lastSize int64 = 0
	var lastSpeed float64 = 0
	var averageSpeed float64 = 0

	for {
		select {
		case <-done:
			stop = true
		default:

			file, err := os.Open(path)
			if err != nil {
				log.Fatal(err)
			}

			fi, err := file.Stat()
			if err != nil {
				log.Fatal(err)
			}

			size := fi.Size()

			if size == 0 {
				size = 1
			}

			var percent float64 = float64(size) / float64(total) * 100

			aSpeed := float64(size-lastSize) / float64((time.Now().UnixMilli() - startTime))

			if size != 0 && (time.Now().UnixMilli()-startTime) != 0 {
				averageSpeed = 0.25*lastSpeed + (1-0.25)*aSpeed
			}

			fmt.Println("Downloading", VideoName, "-", GetFormattedSize(float64(size)), "of", GetFormattedSize(float64(total)), "-", fmt.Sprintf("%.2f", percent)+"%", "-", secondsToMinutes(int64((float64(total-size)/averageSpeed)/1e3)), "Remaining")

			lastSize = size
			lastSpeed = aSpeed
			startTime = time.Now().UnixMilli()
		}

		if stop {
			break
		}

		time.Sleep(time.Millisecond * 2e3)
	}
}

func downloadVideoFromURL(url string, VideoName string, FileName string, Format string, originURL string) {
	file := path.Base(url)

	log.Printf("Downloading %s %s from %s\n", VideoName, file, url)

	InProgressDLs[strings.ReplaceAll(FileName, ".partial", "."+Format)] = url

	var path bytes.Buffer
	path.WriteString(FileName)
	// path.WriteString("/")
	// path.WriteString(file)

	start := time.Now()

	out, err := os.Create(path.String())

	if err != nil {
		os.Remove(FileName)
		log.Println("File GET failed, error:", err)
		return
	}

	defer out.Close()

	headResp, err := http.Head(url)

	if err != nil {
		log.Println("File HEAD failed, error:", err)
		return
	}

	defer headResp.Body.Close()

	size, err := strconv.Atoi(headResp.Header.Get("Content-Length"))

	if err != nil {
		log.Println("File size parse failed, error:", err)
		return
	}

	done := make(chan int64)

	go PrintDownloadPercent(VideoName, done, path.String(), int64(size))

	resp, err := http.Get(url)

	if err != nil {
		os.Remove(FileName)
		log.Println("Ffile GET failed, error:", err)
		return
	}

	defer resp.Body.Close()

	n, err := io.Copy(out, resp.Body)

	if err != nil {
		log.Println("Deleted file", FileName, "due to an error (file copy failed), error:", err)
		return
	}

	done <- n

	elapsed := time.Since(start)

	var errorFlag bool = false

	d, err := os.ReadFile(FileName)
	if err != nil {
		log.Println("Failed to read", FileName, "error:", err)
		errorFlag = true
	}

	if strings.HasPrefix(string(d), "<html>") {
		errorFlag = true
	}

	if errorFlag {
		os.Remove(FileName)
		log.Println("Deleted file", FileName, "due to an error")
		return
	}

	os.Rename(FileName, strings.ReplaceAll(FileName, ".partial", "."+Format))

	// Add to dlindex
	AddDLToIndex(originURL, strings.ReplaceAll(FileName, ".partial", "."+Format))

	log.Printf("Download completed in %s", elapsed)
}

func StartVideoDL(dlURL string, originURL string, alternateVideoExists bool) string {
	log.Println(dlURL)
	dlURLSec := strings.Split(dlURL, "/")
	isDlAMovie := strings.HasPrefix(dlURLSec[4], "m")
	dlFNPrefix := "m"
	dlTitleName := ""
	dlFileName := ""
	if isDlAMovie {
		dlFNPrefix = "m"
	} else {
		dlFNPrefix = "t"
	}
	dlFNPrefix = "media/" + dlFNPrefix
	if _, err := os.Stat("media"); os.IsNotExist(err) {
		os.Mkdir("media", 0777)
	}
	if _, err := os.Stat(dlFNPrefix); os.IsNotExist(err) {
		os.Mkdir(dlFNPrefix, 0777)
	}
	dlModFn := strings.Split(dlURLSec[6], strings.Split(strings.Split(dlURLSec[6], ".")[len(strings.Split(dlURLSec[6], "."))-1], "?")[0]+"?")[0] + "partial"
	dlFileName = dlFNPrefix + "/" + dlModFn
	dlFormat := strings.Split(strings.Split(dlURLSec[6], ".")[len(strings.Split(dlURLSec[6], "."))-1], "?")[0]
	titleRaw := dlURLSec[6]
	titleSec := strings.Split(titleRaw, ".")
	if !isDlAMovie {
		if _, err := os.Stat(dlFNPrefix + "/" + dlURLSec[5]); os.IsNotExist(err) {
			os.Mkdir(dlFNPrefix+"/"+dlURLSec[5], 0777)
		}
		dlFileName = dlFNPrefix + "/" + dlURLSec[5] + "/" + dlModFn
		titleRaw = dlURLSec[5]
	}
	var newDl bool = true
	if _, err := os.Stat(dlFileName); err == nil {
		newDl = false
	} else if _, err := os.Stat(strings.ReplaceAll(dlFileName, ".partial", "."+dlFormat)); err == nil {
		newDl = false
	}
	if !newDl {
		log.Println("Ignoring", dlFileName, "download (already exists or is in progress)")
		if alternateVideoExists {
			// Add other URL to dlindex
			AddDLToIndex(originURL, strings.ReplaceAll(dlFileName, ".partial", "."+dlFormat))
		}
		return "USECACHESERVER/CachedVideo::" + strings.ReplaceAll(dlFileName, ".partial", "."+dlFormat)
	}
	dlTitleName = strings.ReplaceAll(strings.Split(titleRaw, "."+titleSec[len(titleSec)-2])[0], ".", " ")
	if !isDlAMovie {
		dlTitleName += " " + strings.ReplaceAll(dlModFn, ".partial", "")
	}
	log.Println(isDlAMovie, dlURLSec[4], dlTitleName, dlFileName)
	go downloadVideoFromURL(dlURL, dlTitleName, dlFileName, dlFormat, originURL)
	return "USECACHESERVER/CachedVideo::" + strings.ReplaceAll(dlFileName, ".partial", "."+dlFormat)
}

func main() {
	if _, err := os.Stat("dlindex.json"); err == nil {
		d, err := os.ReadFile("dlindex.json")
		if err == nil {
			err := json.Unmarshal(d, &DLIndexes)
			if err != nil {
				log.Println("Failed to unmarshal existing dlindex.json file")
			}
		}
	}

	if _, err := os.Stat("videopersistance.json"); err == nil {
		d, err := os.ReadFile("videopersistance.json")
		if err == nil {
			err := json.Unmarshal(d, &TimePersistance)
			if err != nil {
				log.Println("Failed to unmarshal existing videopersistance.json file")
			}
		}
	}

	http.HandleFunc("/ping", PingHandler)
	http.HandleFunc("/GetVideo", GetVideoHandler)
	http.HandleFunc("/GetPlayer", GetPlayerHandler)
	http.HandleFunc("/SetCurrentTime", SetCurrentTimeHandler)
	http.HandleFunc("/GetVideoPlayer", GetVideoPlayerHandler)
	http.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir("media"))))

	go http.ListenAndServe(":8918", nil)

	log.Println("Started server on port 8918")

	for {
		if shutdown {
			break
		}
	}
}

func extractPlayerURL(URL string) string {
	ctx, cancel := chromedp.NewContext(
		context.Background(),
	)
	defer cancel()

	var body string
	var ok bool

	if err := chromedp.Run(ctx, extractPlayerURLTask(URL, &body, &ok)); err != nil {
		log.Println("Failed to extract video URL from", URL, "error:", err)
	}

	if !ok {
		log.Println("Failed to extract video URL from", URL)
		return ""
	}

	return body
}

func extractPlayerURLTask(urlstr string, body *string, ok *bool) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(urlstr),
		chromedp.WaitEnabled(".btn-success"),
		chromedp.Click(".btn-success"),
		chromedp.WaitReady(".jw-video", chromedp.ByQuery),
		chromedp.AttributeValue(".jw-video", "src", body, ok, chromedp.ByQuery),
	}
}
