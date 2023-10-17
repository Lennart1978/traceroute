package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/aeden/traceroute"
)

const (
	DEFAULT_MAX_HOPS  = 10
	DEFAULT_FIRST_HOP = 1
)

type GeoInfo struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
}

type Details struct {
	ISP string `json:"isp"`
	Org string `json:"org"`
	AS  string `json:"as"`
}

var info GeoInfo

func FillDetails(ip string) (Details, error) {
	var details Details
	resp, err := http.Get(fmt.Sprintf("http://ip-api.com/json/%s", ip))
	if err != nil {
		return details, err
	}
	defer resp.Body.Close()

	var info map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return details, err
	}

	details.ISP = info["isp"].(string)
	details.Org = info["org"].(string)
	details.AS = info["as"].(string)

	return details, nil
}

func getGeoInfo(ip string) GeoInfo {
	url := fmt.Sprintf("http://ip-api.com/json/%s", ip)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return GeoInfo{City: err.Error(), Region: "", Country: ""}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return GeoInfo{City: err.Error(), Region: "", Country: ""}
	}

	var info GeoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		fmt.Println(err)
		return GeoInfo{City: err.Error(), Region: "", Country: ""}
	}
	return info
}

func printHop(hop traceroute.TracerouteHop) string {
	addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
	hostOrAddr := addr
	if hop.Host != "" {
		hostOrAddr = hop.Host
	}
	if hop.Success {
		return fmt.Sprintf("%-3d %v (%v)  %v\n", hop.TTL, hostOrAddr, addr, hop.ElapsedTime)
	} else {
		return fmt.Sprintf("%d - end", hop.TTL)
	}
}

func main() {

	a := app.NewWithID("com.lennart.traceroute")
	a.SetIcon(resourceIconPng)
	w := a.NewWindow("TracerouteGUI v1.1")
	w.Resize(fyne.NewSize(500, 600))
	w.CenterOnScreen()

	Menu := fyne.NewMenu(
		"File",
		fyne.NewMenuItem("About", func() {
			message := "This is a traceroute tool\n2023 by Lennart Martens\nmonkeynator78@gmail.com\nwww.github.com/lennart1978/traceroute"
			dlg := dialog.NewInformation("About", message, w)
			dlg.Show()

		}),
		fyne.NewMenuItem("Quit", func() { a.Quit() }),
	)

	w.SetMainMenu(fyne.NewMainMenu(Menu))

	uid := os.Geteuid()
	if uid == 0 {
		fmt.Println("Running as root, that's fine ! :-)")
	} else {
		fmt.Printf("\nNot running as root !\nPlease run traceroute as root !\n")
		message := "Not running as root !\nPlease run traceroute as root !"
		w.SetContent(container.NewCenter(widget.NewLabel(message)))
		w.ShowAndRun()
		a.Quit()
		os.Exit(1)
	}

	hopsData := binding.BindStringList(&[]string{})
	list := widget.NewListWithData(hopsData, func() fyne.CanvasObject {
		return widget.NewLabel("placeholder")
	}, func(item binding.DataItem, obj fyne.CanvasObject) {
		obj.(*widget.Label).Bind(item.(binding.String))
	})

	list.OnSelected = func(id widget.ListItemID) {
		selectedData, _ := hopsData.Get()
		selectedItem := selectedData[id]
		addr := extractIPFromListItem(selectedItem)
		info = getGeoInfo(addr)
		details, err := FillDetails(addr)
		if err != nil {
			fmt.Println(err)
		}
		message := fmt.Sprintf("\nCity: %s\nRegion: %s\nCountry: %s\nISP: %s\nOrg: %s\n%s", info.City, info.Region, info.Country, details.ISP, details.Org, details.AS)
		dlg := dialog.NewInformation("Details:", message, w)
		dlg.Show()
	}

	//Labels:
	addressLabel := widget.NewLabel("Address :")
	addressLabel.Alignment = fyne.TextAlignCenter
	firstHopLabel := widget.NewLabel("First Hop :")
	maxHopLabel := widget.NewLabel("Max Hops :")
	probesLabel := widget.NewLabel("Probes :")
	routeLabel := widget.NewLabel("Route :")
	routeLabel.Alignment = fyne.TextAlignCenter
	resultLabel := widget.NewLabel("")
	layoutScroll := container.NewScroll(resultLabel)
	layoutScroll.SetMinSize(fyne.NewSize(300, 500))

	//Entrys:
	addressEntry := widget.NewEntry()
	addressEntry.SetText("www.github.com")
	firstHopEntry := widget.NewEntry()
	firstHopEntry.SetText(strconv.Itoa(DEFAULT_FIRST_HOP))
	maxHopEntry := widget.NewEntry()
	maxHopEntry.SetText(strconv.Itoa(DEFAULT_MAX_HOPS))
	probesEntry := widget.NewEntry()
	probesEntry.SetText("1")

	//ProgressBar
	progress := widget.NewProgressBar()

	//Button:
	startButton := widget.NewButton("Start", func() {
		go func() {
			hopsData.Set([]string{})
			m, err := strconv.Atoi(maxHopEntry.Text)
			if err != nil {
				fmt.Println(err)
				m = traceroute.DEFAULT_MAX_HOPS
			}

			f, err := strconv.Atoi(firstHopEntry.Text)
			if err != nil {
				fmt.Println(err)
				f = traceroute.DEFAULT_FIRST_HOP
			}

			q, err := strconv.Atoi(probesEntry.Text)
			if err != nil {
				fmt.Println(err)
				q = 1
			}

			host := addressEntry.Text
			options := traceroute.TracerouteOptions{}
			options.SetRetries(q - 1)
			options.SetMaxHops(m)
			options.SetFirstHop(f)

			ipAddr, err := net.ResolveIPAddr("ip", host)
			if err != nil {
				fmt.Println(err)
			}

			resultLabel.Text = fmt.Sprintf("traceroute to %v (%v), %v hops max, %v byte packets\n", host, ipAddr, options.MaxHops(), options.PacketSize())

			c := make(chan traceroute.TracerouteHop)
			go func() {
				for {
					hop, ok := <-c
					if !ok {
						return
					}
					progress.SetValue(float64(hop.TTL) / float64(options.MaxHops()))

					currentData, _ := hopsData.Get()
					currentData = append(currentData, printHop(hop))
					hopsData.Set(currentData)
				}
			}()

			_, err = traceroute.Traceroute(host, &options, c)
			if err != nil {
				fmt.Printf("Error: %s", err)
			}
		}()
	})

	//Layouts:
	layoutHBox2 := container.NewHBox(firstHopLabel, firstHopEntry, maxHopLabel, maxHopEntry, probesLabel, probesEntry)
	layoutVBoxTop := container.NewVBox(addressLabel, addressEntry, layoutHBox2)
	layoutVBoxBottom := container.NewVBox(progress, startButton)
	layoutBorder := container.NewBorder(layoutVBoxTop, layoutVBoxBottom, nil, nil, list)
	w.SetContent(layoutBorder)
	w.ShowAndRun()
}

func extractIPFromListItem(listItem string) string {
	parts := strings.Split(listItem, " ")
	if len(parts) < 5 || parts[0] == "1" {
		return ""
	} else {
		return strings.Trim(parts[4], "()")
	}
}
