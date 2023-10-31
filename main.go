package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/oschwald/maxminddb-golang" // данные по IP
	"golang.org/x/text/encoding/charmap"   // для смены кодировки
)

// структура для выгрузки информации по сессиям
type SessionsData struct {
	Sessions []struct {
		Client_id     string `json:"client_id"`
		Product_id    string `json:"product_id"`
		Created_on    int64  `json:"created_on"`
		Finished_on   int64  `json:"finished_on"` //or null
		Status        string `json:"status"`
		Creator_ip    string `json:"creator_ip"`
		Abort_comment string `json:"abort_comment"` //or null
		Billing_type  string `json:"billing_type"`  // or null
	}
}

// структура для выгрузки ID игры и названия игры
type Product struct {
	ProductID string `json:"productId"`
	Title     string `json:"title"`
}

// структура для выгрузки ID и названия серверов
type serverManager []struct {
	Uuid    string `json:"uuid"`
	Name    string `json:"name"`
	User_id string `json:"user_id"`
}

type IPInfoResponse struct {
	IP     string `json:"ip"`
	City   string `json:"city"`
	Region string `json:"region"`
	ISP    string `json:"org"`
}

var (
	i, j                 int    = 0, 0 // циклы: j - перебор серверов; i - перебор сессий
	serverName           string        //
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleTitleW = kernel32.NewProc("SetConsoleTitleW")
)

const (
	newTitle = "Drova Session INFO" // Имя окна программы

)

// для получения провайдера
type ASNRecord struct {
	AutonomousSystemNumber       uint32 `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
}

// для получения города
type CityRecord struct {
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Country struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

func main() {
	// Получаем текущую директорию программы
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal("Ошибка получения текущей деректории: ", err, getLine())
	}

	logFilePath := "errors.log" // Имя файла для логирования
	logFilePath = filepath.Join(dir, logFilePath)

	// Открываем файл для записи логов
	logFile, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal("Ошибка открытия файла", err, getLine())
	}
	defer logFile.Close()

	// Устанавливаем файл в качестве вывода для логгера
	log.SetOutput(logFile)

	setConsoleTitle(newTitle) // Устанавливаем новое имя окна

	gameID() // получение списка ID игры - Название игры и сохранение в файл gamesID.txt

	// Указываем файл с токеном
	config := filepath.Join(dir, "config.txt")
	authToken, _ := keyValFile("token", config)                  //  получаем токен
	url1 := "https://services.drova.io/server-manager/servers"   // для получения ID и имени сервера
	url2 := "https://services.drova.io/session-manager/sessions" // для выгрузки сессий

	// Создание HTTP клиента
	client := &http.Client{}

	// Создание нового GET-запроса
	req, err := http.NewRequest("GET", url1, nil)
	if err != nil {
		log.Fatal("Failed to create request: ", err, getLine())
	}
	q := req.URL.Query()
	req.URL.RawQuery = q.Encode()

	// Установка заголовка X-Auth-Token
	req.Header.Set("X-Auth-Token", authToken)

	// Отправка запроса и получение ответа
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Failed to send request: ", err, getLine())
	}
	defer resp.Body.Close()

	// Запись ответа в строку
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		log.Fatal("Failed to write response to buffer: ", err, getLine())
	}
	responseString := buf.String()

	var serv serverManager                        // структура serverManager
	json.Unmarshal([]byte(responseString), &serv) // декодируем JSON файл

	merch := serv[0].User_id  // Получаем ID мерчанта
	currentTime := time.Now() // Получаем текущую дату
	// Создание файла CSV для записи
	csvFile, err := os.Create("sessions_" + merch + " - " + currentTime.Format("02012006-150405") + ".csv")
	if err != nil {
		log.Fatal("Failed to create file: ", err, getLine())
	}
	defer csvFile.Close()
	// Создание CSV-райтера
	writer := csv.NewWriter(charmap.Windows1251.NewEncoder().Writer(csvFile)) // кодировка win1251 для кириллицы
	defer writer.Flush()

	// Запись заголовков столбцов в CSV
	writer.Write([]string{"Станция", "Игра", "IP Клиента", "Город", "Провайдер", "КлиентID", "Статус",
		"Начало сессии", "Конец сессии", "Продолжительность", "Комментарий", "Способ оплаты"})

	// цикл для получения имени и ID всех серверов
	for range serv {
		serverID := serv[j].Uuid
		serverName = serv[j].Name

		// Создание нового GET-запроса
		req, err := http.NewRequest("GET", url2, nil)
		if err != nil {
			log.Fatal("Failed to create request: ", err, getLine())
		}

		// Установка параметров запроса
		q := req.URL.Query()
		q.Add("server_id", serverID)
		req.URL.RawQuery = q.Encode()

		// Установка заголовка X-Auth-Token
		req.Header.Set("X-Auth-Token", authToken)

		// Отправка запроса и получение ответа
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("Failed to send request: ", err, getLine())
		}
		defer resp.Body.Close()

		// Запись ответа в строку
		var buf bytes.Buffer
		_, err = io.Copy(&buf, resp.Body)
		if err != nil {
			log.Fatal("Failed to write response to buffer: ", err, getLine())
		}

		responseString := buf.String()
		var data SessionsData                         // структура SessionsData
		json.Unmarshal([]byte(responseString), &data) // декодируем JSON файл

		fmt.Println(serverName)
		// total := len(data.Sessions)
		// fmt.Println(total)

		i = 0
		for range data.Sessions {
			var sessionOff, sessionDur string = "", ""
			comment := strings.ReplaceAll(data.Sessions[i].Abort_comment, ";", ":")
			sessionOn, _ := dateTimeS(data.Sessions[i].Created_on)
			game, _ := keyValFile(data.Sessions[i].Product_id, "gamesID.txt")

			if data.Sessions[i].Status == "ACTIVE" {
				sessionOff = "session Active" // если сессия активна, так и пишем
			} else { // иначе, записываем значение окончания сессии и высчитываем продолжительность сессии
				sessionOff, _ = dateTimeS(data.Sessions[i].Finished_on)
				_, stopTime := dateTimeS(data.Sessions[i].Finished_on)
				_, startTime := dateTimeS(data.Sessions[i].Created_on)
				sessionDur = dur(stopTime, startTime)
			}

			// собираем инфу по IP

			ip := net.ParseIP(data.Sessions[i].Creator_ip)
			asnRecord, err := getASNRecord(ip)
			if err != nil {
				log.Fatal(err)
			}
			cityRecord, err := getCityRecord(ip)
			if err != nil {
				log.Fatal(err)
			}

			asn := asnRecord.AutonomousSystemOrganization // провайдер клиента
			city := cityRecord.City.Names["en"]           // город клиента

			if city == "" {
				city, asn = ipInf(data.Sessions[i].Creator_ip)
				time.Sleep(1 * time.Second)
			}
			// записываем данные по сессии
			writer.Write([]string{serverName, game, data.Sessions[i].Creator_ip, city, asn, data.Sessions[i].Client_id, data.Sessions[i].Status, sessionOn, sessionOff, sessionDur, comment, data.Sessions[i].Billing_type, ""})
			i++
			fmt.Println(serverName, " - ", i)
			// time.Sleep(1 * time.Millisecond)
		}
		j++
	}
}

// конвертирование даты и времени
func dateTimeS(data int64) (string, time.Time) {

	// Создание объекта времени
	seconds := int64(data / 1000)
	nanoseconds := int64((data % 1000) * 1000000)
	t := time.Unix(seconds, nanoseconds)

	// Форматирование времени
	formattedTime := t.Format("2006-01-02 15:04:05")

	return formattedTime, t
}

func dur(stopTime, startTime time.Time) (sessionDur string) {
	duration := stopTime.Sub(startTime).Round(time.Second)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60
	hou := strconv.Itoa(hours)
	sessionDur = ""
	if hours < 2 {
		sessionDur = sessionDur + "0" + hou + ":"
	} else {
		sessionDur = sessionDur + hou + ":"
	}
	min := strconv.Itoa(minutes)
	if minutes < 10 {
		sessionDur = sessionDur + "0" + min + ":"
	} else {
		sessionDur = sessionDur + min + ":"
	}
	sec := strconv.Itoa(seconds)
	if seconds < 10 {
		sessionDur = sessionDur + "0" + sec
	} else {
		sessionDur = sessionDur + sec
	}
	return
}

func gameID() {
	filedata := "gamesID.txt"
	// Отправить GET-запрос на API
	resp, err := http.Get("https://services.drova.io/product-manager/product/listfull2")
	if err != nil {
		fmt.Println("Ошибка при выполнении запроса:", err, getLine())
		return
	}
	defer resp.Body.Close()

	// Прочитать JSON-ответ
	var products []Product
	err = json.NewDecoder(resp.Body).Decode(&products)
	if err != nil {
		fmt.Println("Ошибка при разборе JSON-ответа:", err, getLine())
		return
	}
	// Создать файл для записи
	file, err := os.Create(filedata)
	if err != nil {
		fmt.Println("Ошибка при создании файла:", err, getLine())
		return
	}
	defer file.Close()

	// Записывать данные в файл
	for _, product := range products {
		line := fmt.Sprintf("%s = %s\n", product.ProductID, product.Title)
		_, err = io.WriteString(file, line)
		if err != nil {
			fmt.Println("Ошибка при записи данных в файл:", err, getLine())
			return
		}
	}
	time.Sleep(1 * time.Second)
}

func keyValFile(keys, fileSt string) (string, error) {
	var val string
	file, err := os.Open(fileSt)
	if err != nil {
		fmt.Println("Ошибка при открытии файла:", err, getLine())
		return "Ошибка при открытии файла:", err
	}
	defer file.Close()

	// Создать сканер для чтения содержимого файла построчно
	scanner := bufio.NewScanner(file)

	// Создать словарь для хранения пары "ключ-значение"
	data := make(map[string]string)

	// Перебирать строки из файла
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " = ")
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			data[key] = value
		}
	}

	if value, ok := data[keys]; ok {
		val = value

	} else {
		val = keys
	}
	return val, err
}

// для смены заголовока программы
func setConsoleTitle(title string) {
	ptrTitle, _ := syscall.UTF16PtrFromString(title)
	_, _, _ = procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(ptrTitle)))
}

// получение строки кода где возникла ошибка
func getLine() int {
	_, _, line, _ := runtime.Caller(1)
	return line
}

// инфо по IP - провайдер
func getASNRecord(ip net.IP) (*ASNRecord, error) {
	db, err := maxminddb.Open("GeoLite2-ASN.mmdb")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var record ASNRecord
	err = db.Lookup(ip, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

// инфо по IP - город
func getCityRecord(ip net.IP) (*CityRecord, error) {
	db, err := maxminddb.Open("GeoLite2-City.mmdb")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var record CityRecord
	err = db.Lookup(ip, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

func ipInf(ip string) (string, string) {
	apiURL := fmt.Sprintf("https://ipinfo.io/%s/json", ip)

	resp, err := http.Get(apiURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var ipInfo IPInfoResponse
	err = json.NewDecoder(resp.Body).Decode(&ipInfo)
	if err != nil {
		log.Fatal(err)
	}

	return ipInfo.City, ipInfo.ISP
}
