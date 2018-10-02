package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

type theta struct {
	ID     string
	Text   string
	Vector []float64
}

var templates = template.Must(template.ParseFiles("tmpl/view.html", "tmpl/index.html"))

var testset, topics = readTheta("theta.csv")
var significant = setSignificance()

const port = ":3737"

func setSignificance() float64 {
	significant := float64(0.1)
	if len(os.Args) > 1 {
		significant, _ = strconv.ParseFloat(os.Args[1], 64)
	}
	fmt.Println("Significance set to", significant)
	fmt.Println("Happy navigating!")
	return significant
}

func readTheta(file string) ([]theta, []string) {
	fmt.Println("Reading file.")
	var topics []string
	f, err := os.Open(file)
	if err != nil {
		fmt.Println("could not open file")
	}
	defer f.Close()
	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	lines, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("error reading all lines: %v", err)
	}
	var result []theta
	for i, line := range lines {
		if i == 0 {
			for i := range line {
				if i < 3 {
					continue
				}
				topics = append(topics, line[i])
			}
			continue
		}
		identifier := line[1]
		text := line[2]
		vector := []float64{}
		for i := range line[3:len(line)] {
			index := i + 3
			floatvalue, _ := strconv.ParseFloat(line[index], 64)
			vector = append(vector, floatvalue)
		}
		result = append(result, theta{ID: identifier, Text: text, Vector: vector})
	}
	fmt.Println("All is read.")
	return result, topics
}

func main() {
	router := mux.NewRouter().StrictSlash(true)
	s := http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))
	js := http.StripPrefix("/js/", http.FileServer(http.Dir("./js/")))
	theta := http.StripPrefix("/cex/", http.FileServer(http.Dir("./theta/")))
	router.PathPrefix("/static/").Handler(s)
	router.PathPrefix("/js/").Handler(js)
	router.PathPrefix("/theta/").Handler(theta)
	// router.HandleFunc("/load/{theta}", LoadDB)
	router.HandleFunc("/view/{urn}/{count}", ViewPage)
	router.HandleFunc("/view/{urn}/{count}/json", ViewPageJs)
	router.HandleFunc("/topic/{topic}/{count}", ViewTopic)
	router.HandleFunc("/", Index)
	log.Println("Listening at" + port + "...")
	log.Fatal(http.ListenAndServe(port, router))
}

// a function to enable CORS on a particular requestion
func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

func Index(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string("Index Page"))

}

func ViewPage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	urn := vars["urn"]
	count := vars["count"]

	info := Info{
		URN:   urn,
		Count: count}

	p, _ := loadPage(info, port)
	renderTemplate(w, "view", p)
}

func ViewPageJs(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	vars := mux.Vars(r)
	urn := vars["urn"]
	count := vars["count"]

	info := Info{
		URN:   urn,
		Count: count}

	p, errorResponse := JsonResponse(info)
	if errorResponse != nil {
		fmt.Fprintln(w, string("error"))
	}

	resultJSON, _ := json.Marshal(p)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ViewTopic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	topic, _ := strconv.Atoi(vars["topic"])
	count, _ := strconv.Atoi(vars["count"])
	topic = topic - 1
	var searchedTopicFloats []float64
	for i := range testset {
		searchedTopicFloats = append(searchedTopicFloats, testset[i].Vector[topic])
	}
	var results []string
	for j := 0; j < count; j++ {
		number := strconv.Itoa(j + 1)
		resultstring1 := ""
		switch j {
		case 0:
			resultstring1 = "Rank " + number + ":"
		default:
			resultstring1 = "\n" + "Rank " + number + ":"
		}
		index := 0
		biggest := searchedTopicFloats[index]
		for i, v := range searchedTopicFloats {
			if v > biggest {
				biggest = v
				index = i
			}
		}
		percentage := "Topic" + vars["topic"] + ": "
		percfloat := testset[index].Vector[topic] * 100
		strnumber := strconv.FormatFloat(percfloat, 'f', 3, 64)
		percentage = percentage + strnumber + " percent"
		resultstring2 := strings.Join([]string{resultstring1, testset[index].ID, percentage, testset[index].Text}, "\n")
		searchedTopicFloats[index] = float64(0)
		results = append(results, resultstring2)
	}
	result := strings.Join(results, "\n")
	fmt.Fprintf(w, result)
}

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func findURN(urn string) int {
	var result int
	for i := range testset {
		if testset[i].ID == urn {
			result = i
		}
	}
	return result
}

func loadPage(info Info, port string) (*Page, error) {
	urn := info.URN
	position := findURN(urn)
	rank := info.Count
	number, err := strconv.Atoi(rank)
	check(err)
	number = number + 1
	best := ""
	text := ""

	result := testmanhattan(testset[position], testset)
	sorted_result := sortresults(result, number)

	var resultNetwork Network
	var texts []string
	var ids []string
	var manhattans []string
	var bests []string
	var signis []string

	for i := range sorted_result {
		thebest := ""
		signi := ""
		index := floatfind(result, sorted_result[i])
		switch {
		case i == 0:
			mandist := "0"
			texts = append(texts, testset[index].Text)
			ids = append(ids, testset[index].ID)
			manhattans = append(manhattans, mandist)
			sorted_indiresult := reversesortresults(testset[index].Vector, 3)
			for j := range sorted_indiresult {
				indi_index := floatfind(testset[index].Vector, sorted_indiresult[j])
				normed := testset[index].Vector[indi_index] * 100
				beststring := "Topic" + strconv.Itoa(indi_index+1) + " " + topics[indi_index] + ": " + strconv.FormatFloat(normed, 'f', 2, 64) + "%" + "</br>"
				best = best + beststring
			}
			text = testset[index].Text
			signi = "Your Passage"
			signis = append(signis, signi)
			bests = append(bests, best)
			resultNetwork.Nodes = append(resultNetwork.Nodes, Node{ID: urn, Label: urn, X: float64(1), Y: float64(1), Size: float64(1)})
		case i > 0:
			mannormed := sorted_result[i] * 100
			mandist := strconv.FormatFloat(mannormed, 'f', 2, 64)
			texts = append(texts, testset[index].Text)
			ids = append(ids, testset[index].ID)
			manhattans = append(manhattans, mandist)
			sorted_indiresult := reversesortresults(testset[index].Vector, 3)
			for j := range sorted_indiresult {
				indi_index := floatfind(testset[index].Vector, sorted_indiresult[j])
				normed := testset[index].Vector[indi_index] * 100
				beststring := "Topic" + strconv.Itoa(indi_index+1) + " " + topics[indi_index] + ": " + strconv.FormatFloat(normed, 'f', 2, 64) + "%" + "</br>"
				thebest = thebest + beststring
			}
			for j := range testset[index].Vector {
				topicdistance := mpair(testset[index].Vector[j], testset[position].Vector[j])
				if topicdistance > significant {
					topicdistance = topicdistance * 100
					signistring := "Distance Topic to " + strconv.Itoa(j+1) + " " + topics[j] + ": " + strconv.FormatFloat(topicdistance, 'f', 2, 64) + "%" + "</br>"
					signi = signi + signistring
				}
			}
			signis = append(signis, signi)
			bests = append(bests, thebest)
			xcord := float64(1) + float64(1)*sorted_result[i]
			ycord := float64(1) + float64(-1)*sorted_result[i]
			var size float64
			size = float64(1) * (float64(1) - sorted_result[i])
			resultNetwork.Nodes = append(resultNetwork.Nodes, Node{ID: testset[index].ID, Label: testset[index].ID, X: xcord, Y: ycord, Size: size})
			edgeID := "edge" + strconv.Itoa(i)
			resultNetwork.Edges = append(resultNetwork.Edges, Edge{ID: edgeID, Source: urn, Target: testset[index].ID})
		}
	}
	createNetwork(resultNetwork)
	distance := strconv.FormatFloat(significant, 'f', -1, 64)
	for i := range texts {
		texts[i] = "\"" + texts[i] + "\""
		ids[i] = "\"" + ids[i] + "\""
		manhattans[i] = "\"" + manhattans[i] + "\""
		bests[i] = "\"" + bests[i] + "\""
		signis[i] = "\"" + signis[i] + "\""
	}
	jScript := strings.Join(texts, ",")
	jSIDs := strings.Join(ids, ",")
	jsDistance := strings.Join(manhattans, ",")
	jsBest := strings.Join(bests, ",")
	jsSigni := strings.Join(signis, ",")
	return &Page{URN: urn, Distance: distance, BestTopics: template.HTML(best), Text: text, Port: port, JSTexts: template.JS(jScript), JSIDs: template.JS(jSIDs), JSDistance: template.JS(jsDistance), JSBest: template.JS(jsBest), JSSigni: template.JS(jsSigni)}, nil
}

func createNetwork(resultNetwork Network) {
	networkJSON, err := json.Marshal(resultNetwork)
	check(err)

	filename := "static/data2.json"
	ioutil.WriteFile(filename, networkJSON, 0600)

	return
}

func JsonResponse(info Info) (PassageJsonResponse, error) {

	urn := info.URN
	position := findURN(urn)
	rank := info.Count
	number, err := strconv.Atoi(rank)
	check(err)
	number = number + 1
	text := ""

	result := testmanhattan(testset[position], testset)
	sorted_result := sortresults(result, number)

	var texts []string
	var ids []string
	var manhattans []string

	for i := range sorted_result {
		index := floatfind(result, sorted_result[i])
		switch {
		case i == 0:
			mandist := "0"
			texts = append(texts, testset[index].Text)
			ids = append(ids, testset[index].ID)
			manhattans = append(manhattans, mandist)
			text = testset[index].Text
		case i > 0:
			mannormed := sorted_result[i] * 100
			mandist := strconv.FormatFloat(mannormed, 'f', 2, 64)
			texts = append(texts, testset[index].Text)
			ids = append(ids, testset[index].ID)
			manhattans = append(manhattans, mandist)
		}
	}

	relatedItems := []relatedItem{}
	for i := range texts {
		relatedItems = append(relatedItems, relatedItem{Id: ids[i], Distance: manhattans[i]})
	}

	passageObject := PassageJsonResponse{URN: "test", Text: text, Items: relatedItems}
	return passageObject, nil
}

type Network struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Size  float64 `json:"size"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type Info struct {
	URN, Count string
}

type Page struct {
	URN        string
	Distance   string
	BestTopics template.HTML
	Text       string
	Port       string
	JSTexts    template.JS
	JSIDs      template.JS
	JSDistance template.JS
	JSBest     template.JS
	JSSigni    template.JS
}

func mpair(x, y float64) float64 {
	switch {
	case y < x:
		return x - y
	case x < y:
		return y - x
	default:
		return 0
	}
}

func manhattan(x, y []float64) float64 {
	var result float64
	for i := range x {
		result = result + mpair(x[i], y[i])
	}
	return result
}

func manhattan_wghted(x, y, weight []float64) float64 {
	var result float64
	for i := range x {
		result = result + mpair(x[i], y[i])*weight[i]
	}
	return result
}

func testmanhattan(query theta, dataset []theta) []float64 {
	var result []float64
	for i := range dataset {
		result = append(result, manhattan(query.Vector, dataset[i].Vector))
	}
	return result
}

func sortresults(result []float64, number int) []float64 {
	var sorted_result []float64
	for i := range result {
		sorted_result = append(sorted_result, result[i])
	}
	sort.Float64s(sorted_result)
	sorted_result = sorted_result[0:number]
	return sorted_result
}

func reversesortresults(result []float64, number int) []float64 {
	var sorted_result []float64
	for i := range result {
		sorted_result = append(sorted_result, result[i])
	}
	sort.Float64s(sorted_result)
	for i := len(sorted_result)/2 - 1; i >= 0; i-- {
		opp := len(sorted_result) - 1 - i
		sorted_result[i], sorted_result[opp] = sorted_result[opp], sorted_result[i]
	}
	sorted_result = sorted_result[0:number]
	return sorted_result
}

func floatfind(slice []float64, value float64) int {
	for p, v := range slice {
		if v == value {
			return p
		}
	}
	return -1
}

type PassageJsonResponse struct {
	URN   string        `json:"urn"`
	Text  string        `json:"text"`
	Items []relatedItem `json:"items"`
}
type relatedItem struct {
	Id       string `json:"id"`
	Distance string `json:"distance"`
}
