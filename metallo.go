package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
)

type theta struct {
	ID     string
	Text   string
	Vector []float64
}

type serverConfig struct {
	Host         string  `json:"host"`
	Port         string  `json:"port"`
	Source       string  `json:"csv_source"`
	Local        bool    `json:"local"`
	Significance float64 `json:"significance"`
}

var templates = template.Must(template.ParseFiles("tmpl/view.html", "tmpl/index.html"))

var confvar = loadConfiguration("./config.json")
var topics = []string{}
var significant = confvar.Significance
var port = confvar.Port
var host = confvar.Host
var address = host + port
var pwd, _ = os.Getwd()
var dbname = pwd + "/metallo.db"

func retrieveTopics() (topics []string) {
	db, err := bolt.Open(dbname, 0644, nil)
	check(err)
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("topics"))
		if bucket == nil {
			return fmt.Errorf("bucket not found")
		}
		val := bucket.Get([]byte("topics"))
		topics, _ = gobDecodeTopics(val)
		return nil
	})
	db.Close()
	return topics
}

func gobEncode(p interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(p)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gobDecode(data []byte) (theta, error) {
	var p *theta
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&p)
	if err != nil {
		return theta{}, err
	}
	return *p, nil
}

func gobDecodeTopics(data []byte) ([]string, error) {
	var p *[]string
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&p)
	if err != nil {
		return []string{}, err
	}
	return *p, nil
}

func thetaToDB(thetafile theta) error {
	dbkey := []byte(thetafile.ID)
	dbvalue, err := gobEncode(&thetafile)

	db, err := bolt.Open(dbname, 0644, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("theta"))
		if err != nil {
			return err
		}
		val := bucket.Get(dbkey)
		if val != nil {
			return errors.New("work exists already")
		}
		err = bucket.Put(dbkey, dbvalue)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func readTheta() []string {
	os.Remove(dbname)
	file := confvar.Source
	fmt.Println("Reading file.")
	var topics []string
	switch confvar.Local {
	case false:
		fmt.Println("Fetching external resource.")
		data, _ := getContent(file)
		str := string(data)
		reader := csv.NewReader(strings.NewReader(str))
		reader.LazyQuotes = true
		lines, err := reader.ReadAll()
		if err != nil {
			log.Fatalf("error reading all lines: %v", err)
		}

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
			thetaToDB(theta{ID: identifier, Text: text, Vector: vector})
		}
		fmt.Println("All is read.")
	case true:
		fmt.Println("Fetching internal resource.")
		f, err := os.Open(file)
		if err != nil {
			fmt.Println("could not open file")
		}
		defer f.Close()
		reader := csv.NewReader(bufio.NewReader(f))
		reader.LazyQuotes = true

		linecount := 0
		recordcount := 0
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if linecount == 0 {
				for j := range record {
					if j < 3 {
						continue
					}
					topics = append(topics, record[j])
				}
				linecount++
				continue
			}
			identifier := record[1]
			text := record[2]
			vector := []float64{}
			for j := range record[3:len(record)] {
				index := j + 3
				floatvalue, _ := strconv.ParseFloat(record[index], 64)
				vector = append(vector, floatvalue)
			}
			thetaToDB(theta{ID: identifier, Text: text, Vector: vector})
			recordcount++
			fmt.Printf("\rWrote %d records to the database.", recordcount)
		}
		fmt.Println("All is read and written.")
	}

	dbkey := []byte("topics")
	dbvalue, err := gobEncode(&topics)
	db, err := bolt.Open(dbname, 0644, nil)
	check(err)
	defer db.Close()
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("topics"))
		if err != nil {
			return err
		}
		err = bucket.Put(dbkey, dbvalue)
		if err != nil {
			return err
		}
		return nil
	})
	check(err)
	return topics
}

func main() {
	loadDB := flag.Bool("loadDB", false, "load DB from CSV")
	flag.Parse()
	if *loadDB {
		fmt.Println("(Re-)building the db...")
		topics = readTheta()
	} else {
		fmt.Println("Starting without re-building the db...")
		topics = retrieveTopics()
	}
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

func loadConfiguration(file string) serverConfig {
	var config serverConfig
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return config
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
	count, _ := strconv.Atoi(vars["count"])
	info := Info{
		URN:   urn,
		Count: count}

	p, _ := loadPage(info, address)
	renderTemplate(w, "view", p)
}

func ViewPageJs(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	vars := mux.Vars(r)
	urn := vars["urn"]
	count, _ := strconv.Atoi(vars["count"])

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
	thetas := make([]theta, count)
	topic = topic - 1
	db, err := bolt.Open(dbname, 0644, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.View(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		b := tx.Bucket([]byte("theta"))

		c := b.Cursor()
		indexcount := 0

		for k, v := c.First(); k != nil; k, v = c.Next() {
			newtheta, _ := gobDecode(v)
			if indexcount < count {
				thetas[indexcount] = newtheta
				indexcount++
				continue
			}
			minindex, minfloat := minIndexTheta(thetas, topic)
			if newtheta.Vector[topic] > minfloat {
				thetas[minindex] = newtheta
			}
			indexcount++
		}
		return nil
	})
	var results []string

	for i := range thetas {
		resultstring1 := ""
		switch i {
		case 0:
			resultstring1 = "Rank " + strconv.Itoa(i+1) + ":"
		default:
			resultstring1 = "\n" + "Rank " + strconv.Itoa(i+1) + ":"
		}
		maxindex, maxfloat := maxIndexTheta(thetas, topic)
		percentage := "Topic" + vars["topic"] + ": "
		percfloat := maxfloat * 100
		strnumber := strconv.FormatFloat(percfloat, 'f', 3, 64)
		percentage = percentage + strnumber + " percent"
		resultstring2 := strings.Join([]string{resultstring1, thetas[maxindex].ID, percentage, thetas[maxindex].Text}, "\n")
		thetas[maxindex].Vector[topic] = float64(0)
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

func loadPage(info Info, address string) (*Page, error) {
	urn := info.URN
	query := theta{}
	db, err := bolt.Open(dbname, 0644, nil)
	check(err)
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("theta"))
		if bucket == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		val := bucket.Get([]byte(urn))
		query, _ = gobDecode(val)
		return nil
	})
	db.Close()
	thetas, distances := testmanhattan(query, info.Count)
	best := ""
	text := ""

	var resultNetwork Network
	var texts []string
	var ids []string
	var manhattans []string
	var bests []string
	var signis []string

	for i := range thetas {
		thebest := ""
		signi := ""
		switch {
		case i == 0:
			texts = append(texts, thetas[i].Text)
			ids = append(ids, thetas[i].ID)
			manhattans = append(manhattans, "0")
			sortedIndiresult := reversesortresults(thetas[i].Vector, 3)
			for j := range sortedIndiresult {
				indiIndex := sortedIndiresult[j]
				normed := thetas[i].Vector[indiIndex] * 100
				if normed > 5 {
					beststring := "Topic" + strconv.Itoa(indiIndex+1) + " " + topics[indiIndex] + ": " + strconv.FormatFloat(normed, 'f', 2, 64) + "%" + "</br>"
					best = best + beststring
				}
			}
			text = thetas[i].Text
			signi = "Your Passage"
			signis = append(signis, signi)
			bests = append(bests, best)
			resultNetwork.Nodes = append(resultNetwork.Nodes, Node{ID: urn, Label: urn, X: float64(1), Y: float64(1), Size: float64(1)})
		case i > 0:
			mannormed := distances[i] * 100
			mandist := strconv.FormatFloat(mannormed, 'f', 2, 64)
			texts = append(texts, thetas[i].Text)
			ids = append(ids, thetas[i].ID)
			manhattans = append(manhattans, mandist)
			sortedIndiresult := reversesortresults(thetas[i].Vector, 3)
			for j := range sortedIndiresult {
				indiIndex := sortedIndiresult[j]
				normed := thetas[i].Vector[indiIndex] * 100
				if normed > 5 {
					beststring := "Topic" + strconv.Itoa(indiIndex+1) + " " + topics[indiIndex] + ": " + strconv.FormatFloat(normed, 'f', 2, 64) + "%" + "</br>"
					thebest = thebest + beststring
				}
			}
			for j := range thetas[i].Vector {
				topicdistance := mpair(thetas[i].Vector[j], query.Vector[j])
				if topicdistance > significant {
					topicdistance = topicdistance * 100
					signistring := "Distance Topic to " + strconv.Itoa(j+1) + " " + topics[j] + ": " + strconv.FormatFloat(topicdistance, 'f', 2, 64) + "%" + "</br>"
					signi = signi + signistring
				}
			}
			signis = append(signis, signi)
			bests = append(bests, thebest)
			xcord := float64(1) + float64(1)*distances[i]
			ycord := float64(1) + float64(-1)*distances[i]
			var size float64
			size = float64(1) * (float64(1) - distances[i])
			resultNetwork.Nodes = append(resultNetwork.Nodes, Node{ID: thetas[i].ID, Label: thetas[i].ID, X: xcord, Y: ycord, Size: size})
			edgeID := "edge" + strconv.Itoa(i)
			resultNetwork.Edges = append(resultNetwork.Edges, Edge{ID: edgeID, Source: urn, Target: thetas[i].ID})
		}
	}
	networkJSON, _ := json.Marshal(resultNetwork)
	stringJSON := template.JS(string(networkJSON))
	distance := strconv.FormatFloat(significant, 'f', -1, 64)
	for i := range texts {
		texts[i] = strings.Replace(texts[i], "\"", "'", -1)
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
	return &Page{URN: urn, Distance: distance, BestTopics: template.HTML(best), Text: text, Address: address, Port: port, Host: host, JSON: stringJSON, JSTexts: template.JS(jScript), JSIDs: template.JS(jSIDs), JSDistance: template.JS(jsDistance), JSBest: template.JS(jsBest), JSSigni: template.JS(jsSigni)}, nil
}

func JsonResponse(info Info) (PassageJsonResponse, error) {
	urn := info.URN
	query := theta{}
	db, err := bolt.Open(dbname, 0644, nil)
	check(err)
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("theta"))
		if bucket == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		val := bucket.Get([]byte(urn))
		query, _ = gobDecode(val)
		return nil
	})
	db.Close()
	thetas, distances := testmanhattan(query, info.Count)
	text := ""
	var ids []string
	var manhattans []string

	for i := range thetas {
		switch {
		case i == 0:
			ids = append(ids, thetas[i].ID)
			manhattans = append(manhattans, "0")
			text = thetas[i].Text
		case i > 0:
			mannormed := distances[i] * 100
			mandist := strconv.FormatFloat(mannormed, 'f', 2, 64)
			ids = append(ids, thetas[i].ID)
			manhattans = append(manhattans, mandist)
		}
	}

	relatedItems := []relatedItem{}
	for i := range ids {
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
	URN   string
	Count int
}

type Page struct {
	URN        string
	Distance   string
	BestTopics template.HTML
	Text       string
	Port       string
	Host       string
	Address    string
	JSON       template.JS
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

func minIndexTheta(thetas []theta, topic int) (index int, floatvalue float64) {
	index = 0
	floatvalue = thetas[index].Vector[topic]
	for i, e := range thetas {
		if e.Vector[topic] < floatvalue {
			index = i
			floatvalue = e.Vector[topic]
		}
	}
	return
}

func maxIndexTheta(thetas []theta, topic int) (index int, floatvalue float64) {
	index = 0
	floatvalue = thetas[index].Vector[topic]
	for i := range thetas {
		if thetas[i].Vector[topic] > floatvalue {
			index = i
			floatvalue = thetas[i].Vector[topic]
		}
	}
	return
}

func manhattan_wghted(x, y, weight []float64) float64 {
	var result float64
	for i := range x {
		result = result + mpair(x[i], y[i])*weight[i]
	}
	return result
}

func maxIndexDistance(distances []float64) (index int, floatvalue float64) {
	index = 0
	floatvalue = distances[index]
	for i := range distances {
		if distances[i] > floatvalue {
			index = i
			floatvalue = distances[i]
		}
	}
	return
}

type dataframe struct {
	Thetas    []theta
	Distances []float64
}

func (m dataframe) Len() int           { return len(m.Distances) }
func (m dataframe) Less(i, j int) bool { return m.Distances[i] < m.Distances[j] }
func (m dataframe) Swap(i, j int) {
	m.Thetas[i], m.Thetas[j] = m.Thetas[j], m.Thetas[i]
	m.Distances[i], m.Distances[j] = m.Distances[j], m.Distances[i]
}

func testmanhattan(query theta, count int) ([]theta, []float64) {
	thetas := make([]theta, count+1)
	distances := make([]float64, count+1)
	db, err := bolt.Open(dbname, 0644, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("theta"))

		c := b.Cursor()
		indexcount := 0

		for k, v := c.First(); k != nil; k, v = c.Next() {
			newtheta, err := gobDecode(v)
			if err != nil {
				fmt.Println("decoding problem")
			}
			if indexcount <= count {
				thetas[indexcount] = newtheta
				distances[indexcount] = manhattan(query.Vector, newtheta.Vector)
				indexcount++
				continue
			}
			maxindex, maxfloat := maxIndexDistance(distances)
			newdistance := manhattan(query.Vector, newtheta.Vector)
			if newdistance < maxfloat {
				thetas[maxindex] = newtheta
				distances[maxindex] = newdistance
			}
			indexcount++
		}
		return nil
	})
	sort.Sort(dataframe{Thetas: thetas, Distances: distances})
	return thetas, distances
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

func reversesortresults(result []float64, number int) []int {
	sorted_result := make([]int, number)
	sortedFloats := make([]float64, number)
	lowestfloatindex := 0
	lowestfloat := float64(0)
	for i := range result {
		if i < number {
			sorted_result[i] = i
			sortedFloats[i] = result[i]
			if i == 0 {
				lowestfloat = result[i]
				lowestfloatindex = i
			}
			if i != 0 {
				if result[i] < lowestfloat {
					lowestfloat = result[i]
					lowestfloatindex = i
				}
			}
			continue
		}
		if result[i] > lowestfloat {
			sorted_result[lowestfloatindex] = i
			sortedFloats[lowestfloatindex] = result[i]
			lowestfloatindex, lowestfloat = maxIndexDistance(sortedFloats)
		}
	}
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

func getContent(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status error: %v", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Read body: %v", err)
	}

	return data, nil
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
