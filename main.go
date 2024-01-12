package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/bogem/id3v2"
	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

type Track struct {
	Author, NameOfTrack, Time *widget.Label
	Button                    *widget.Button
	Path                      string
	streamer                  beep.StreamSeekCloser
	ctrl                      *beep.Ctrl
	format                    beep.Format
}

func saveTracks(tracks []string, filePath string) error {
	data, err := json.Marshal(tracks)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filePath, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func loadTracks(filePath string) ([]string, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var tracks []string
	err = json.Unmarshal(data, &tracks)
	if err != nil {
		return nil, err
	}

	return tracks, nil
}

var speakerInitialized = false

func Duration(path string) (string, float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}

	streamer, format, err := mp3.Decode(f)
	if err != nil {
		return "", 0, err
	}
	defer streamer.Close()

	if !speakerInitialized {
		speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
		speakerInitialized = true
	}

	seconds := float64(streamer.Len()) / float64(format.SampleRate)
	second := float64(streamer.Len()) / float64(format.SampleRate)

	duration := time.Duration(seconds) * time.Second

	minutes := int(duration.Minutes())
	seconds = float64(int(duration.Seconds()) % 60)

	timeString := fmt.Sprintf("%d:%02d", minutes, int(seconds))

	return timeString, second, nil
}

var Slider *widget.Slider

var isPlaying bool
var currentTrack *Track

func NewTrack(author, name, duration, path string, seconds float64, Slider *widget.Slider, s beep.StreamSeekCloser, fm beep.Format) *Track {
	t := &Track{
		Author:      widget.NewLabel(author + "\t —"),
		NameOfTrack: widget.NewLabel(name),
		Time:        widget.NewLabel(duration),
		Path:        path,
		format:      fm,
		streamer:    s,
	}

	t.Button = widget.NewButton("Play", func() {
		Slider.Max = seconds

		if currentTrack != nil && currentTrack.ctrl != nil {
			if currentTrack != t {
				currentTrack.ctrl.Paused = true
				currentTrack.streamer.Seek(0)
				isPlaying = false
			}
		}

		if !isPlaying {
			currentTrack = t
			if t.ctrl == nil {
				t.ctrl = &beep.Ctrl{Streamer: beep.Loop(-1, t.streamer)}
				//t.ctrl = &beep.Ctrl{Streamer: t.streamer}

				speaker.Play(t.ctrl)

				Slider.OnChanged = func(value float64) {
					if t.streamer != nil {
						currentTrack.streamer.Seek(currentTrack.format.SampleRate.N(time.Second) * int(value))
					}
				}

				go updateSliderAndConsole(t, Slider)

			}

			t.ctrl.Paused = false
			isPlaying = true

		} else {
			t.ctrl.Paused = true
			isPlaying = false
		}
	})

	return t
}

func updateSliderAndConsole(t *Track, Slider *widget.Slider) {
	for {
		nowsec := int64(float64(currentTrack.streamer.Position()) / float64(currentTrack.format.SampleRate))
		sec := int64(float64(currentTrack.streamer.Len()) / float64(currentTrack.format.SampleRate))
		Slider.Max = float64(sec)
		Slider.Value = float64(currentTrack.streamer.Position()) / float64(currentTrack.format.SampleRate)
		Slider.Refresh()
		time.Sleep(time.Second)
		fmt.Println(nowsec, " / ", sec, "  ", currentTrack.NameOfTrack.Text)
		if currentTrack.ctrl != nil && !currentTrack.ctrl.Paused {

			if (nowsec - 1) >= (sec - 3) {
				// Включаем следующий трек
				fmt.Println(currentTrack.NameOfTrack.Text)
				currentTrack.ctrl.Paused = true
				PlayNextTrack()

				break
			}
		}
	}
}

func (t *Track) Container() *fyne.Container {
	return container.NewVBox(container.NewHBox(t.Button, t.Author, t.NameOfTrack, t.Time))
}

func PlayNextTrack() {
	// Находим текущий индекс трека
	currentIndex := -1
	for i, track := range TrackListPath {
		if track == currentTrack.Path {
			currentIndex = i
			break
		}
	}
	fmt.Println(currentTrack.NameOfTrack.Text)
	// Переключаемся на следующий трек
	if currentIndex != -1 && currentIndex < len(TrackListPath)-1 {
		if currentIndex == (len(TrackListPath) - 1) {
			TrackStructList[0].Button.OnTapped()
			currentTrack = TrackStructList[0] // Обновляем currentTrack
		} else {
			TrackStructList[currentIndex+1].Button.OnTapped()
			currentTrack = TrackStructList[currentIndex+1]
		}
	}
	currentTrack = TrackStructList[currentIndex+1]
	fmt.Println(currentTrack.NameOfTrack.Text)
	currentIndex = -1
}

func PlayTrack(track *Track) {
	currentTrack.Button.OnTapped()
}

var TrackListPath []string
var TrackStructList []*Track

func main() {

	Slider := widget.NewSlider(0, 100)

	a := app.New()
	w := a.NewWindow("MP3 Player")
	w.Resize(fyne.NewSize(400, 500))

	content := container.NewVBox()
	tracks, err := loadTracks("tracks.json")
	if err != nil {
		fmt.Println("Error loading tracks:", err)
	} else {
		TrackListPath = tracks
		fileInfo, err := os.Stat("tracks.json")
		if err != nil {
			fmt.Println("Error getting file info:", err)
		}
		if fileInfo.Size() != 0 {
			for _, filePath := range tracks {
				mp3File, err := os.Open(filePath)
				if err != nil {
					fmt.Println("Error opening file:", err)
					return
				}
				defer mp3File.Close()

				// Читаем теги ID3v2
				tag, err := id3v2.Open(mp3File.Name(), id3v2.Options{Parse: true})
				if err != nil {
					fmt.Println("Error parsing ID3:", err)
					return
				}
				defer tag.Close()

				duration, seconds, err := Duration(filePath)
				if err != nil {
					fmt.Println("Error decode file:", err)
				}

				streamer, format, err := mp3.Decode(mp3File)
				if err != nil {
					fyne.LogError("Failed to decode file", err)
					return
				}

				track := NewTrack(tag.Artist(), tag.Title(), duration, filePath, seconds, Slider, streamer, format)
				TrackStructList = append(TrackStructList, track)
				content.Add(track.Container())
				content.Refresh()
			}
		}
		content.Refresh()
	}

	TrackList := container.NewVScroll(content)
	TrackList.SetMinSize(fyne.NewSize(400, 300))

	addTrackButton := widget.NewButton("Дабавить трек", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				fmt.Println("Ошибка при открытии файла:", err)
				return
			}
			if reader == nil {
				return
			}
			filePath := reader.URI().Path()

			mp3File, err := os.Open(filePath)
			if err != nil {
				fmt.Println("Error opening file:", err)
				return
			}
			defer mp3File.Close()

			// Читаем теги ID3v2
			tag, err := id3v2.Open(mp3File.Name(), id3v2.Options{Parse: true})
			if err != nil {
				fmt.Println("Error parsing ID3:", err)
				return
			}
			defer tag.Close()

			duration, seconds, err := Duration(filePath)
			if err != nil {
				fmt.Println("Error decode file:", err)
			}

			streamer, format, err := mp3.Decode(mp3File)
			if err != nil {
				fyne.LogError("Failed to decode file", err)
				return
			}

			tracks = append(tracks, filePath)
			err = saveTracks(tracks, "tracks.json")
			if err != nil {
				fmt.Println("Error save Tracks")
			}

			track := NewTrack(tag.Artist(), tag.Title(), duration, filePath, seconds, Slider, streamer, format)
			content.Add(track.Container())
			content.Refresh()
		}, w)

		fd.Show()
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("Ваша музыка"),
		TrackList,
		addTrackButton,
		Slider,
	))
	w.ShowAndRun()
}
