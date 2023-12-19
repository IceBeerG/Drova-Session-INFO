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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/oschwald/maxminddb-golang" // данные по IP
	"golang.org/x/sys/windows/registry"
)

type Release struct {
	PublishedAt time.Time `json:"published_at"`
}

// структура для выгрузки информации по сессиям
type SessionsData struct {
	Sessions []struct {
		Id            int32  `json:"id"`
		Uuid          string `json:"uuid"`
		Client_id     string `json:"client_id"`
		Server_id     string `json:"server_id"`
		Product_id    string `json:"product_id"`
		Created_on    int64  `json:"created_on"`
		Finished_on   int64  `json:"finished_on"` //or null
		Status        string `json:"status"`
		Creator_ip    string `json:"creator_ip"`
		Abort_comment string `json:"abort_comment"` //or null
		Score         string `json:"score"`         //or null
		Score_reason  string `json:"score_reason"`  //or null
		Comment       string `json:"score_text"`    //or null
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
	i, j                          int    = 0, 0 // циклы: j - перебор серверов; i - перебор сессий
	serverName, mmdbASN, mmdbCity string        //
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleTitleW          = kernel32.NewProc("SetConsoleTitleW")
)

const (
	newTitle = "Drova Session INFO" // Имя окна программы

)

// для получения провайдера в оффлайн базе
type ASNRecord struct {
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
}

// для получения города региона в оффлайн базе
type CityRecord struct {
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Subdivision []struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

func main() {
	var authToken string

	// Получаем текущую директорию программы
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal("Ошибка получения текущей деректории: ", err, getLine())
	}

	logFilePath := "errors.log" // Имя файла для логирования
	logFilePath = filepath.Join(dir, logFilePath)
	mmdbASN = filepath.Join(dir, "GeoLite2-ASN.mmdb")   // файл оффлайн базы IP. Провайдер
	mmdbCity = filepath.Join(dir, "GeoLite2-City.mmdb") // файл оффлайн базы IP. Город и область

	_, err = os.Stat(mmdbASN)
	if os.IsNotExist(err) {
		// Файл не существует
		log.Printf("[INFO] Файл %s отсутствует\n", mmdbASN)
	} else {
		updateGeoLite(mmdbASN, mmdbASN)
	}

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
	fileConfig := filepath.Join(dir, "config.txt")
	_, err = os.Stat(fileConfig)
	if os.IsNotExist(err) {
		// Файл не существует
		log.Printf("[INFO] Файл %s отсутствует\n", fileConfig)
		regFolder := `SOFTWARE\ITKey\Esme`
		serverID := regGet(regFolder, "last_server") // получаем ID сервера
		regFolder += `\servers\` + serverID
		authToken = regGet(regFolder, "auth_token") // получаем токен для авторизации
		log.Println(authToken)
	} else {
		authToken, _ = keyValFile("token", fileConfig) //  получаем токен
	}

	// authToken := "c906fd81-d613-4b0e-9e68-6715076a654d"
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
	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	// Запись заголовков столбцов в CSV
	writer.Write([]string{"ID сессии", "Станция", "Игра", "IP Клиента", "Город", "Регион", "Провайдер", "КлиентID", "Статус",
		"Начало сессии", "Конец сессии", "Продолжительность", "Продолжительность UNIX", "Abort_comment", "Score", "Score_reason", "Комментарий", "Способ оплаты"})

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

		i = 0
		z := 0
		for range data.Sessions {
			var sessionOff, sessionDur, sessionDurUnix string = "", "", ""
			var city, region, asn string = "", "", ""

			comment := strings.ReplaceAll(data.Sessions[i].Comment, `,`, ".")
			comment = strings.ReplaceAll(comment, "\n", "")
			sessionOn, _ := dateTimeS(data.Sessions[i].Created_on)
			game, _ := keyValFile(data.Sessions[i].Product_id, "gamesID.txt")
			idsession := data.Sessions[i].Uuid

			if data.Sessions[i].Status == "ACTIVE" {
				sessionOff = "session Active" // если сессия активна, так и пишем
				sessionDur = "session Active"
				sessionDurUnix = "session Active"
			} else { // иначе, записываем значение окончания сессии и высчитываем продолжительность сессии
				sessionOff, _ = dateTimeS(data.Sessions[i].Finished_on)
				_, stopTime := dateTimeS(data.Sessions[i].Finished_on)
				_, startTime := dateTimeS(data.Sessions[i].Created_on)
				sessionDur = dur(stopTime, startTime)
				sessionDurU := data.Sessions[i].Finished_on - data.Sessions[i].Created_on
				sessionDurUnix = strconv.FormatInt((sessionDurU), 10)
			}

			_, err = os.Stat(mmdbASN)
			if !os.IsNotExist(err) {
				city, region, asn = offlineDBip(data.Sessions[i].Creator_ip)

			} else if os.IsNotExist(err) && z != 0 {
				// Файл не существует
				log.Printf("[INFO] Файл %s отсутствует\n", fileConfig)
				z++
			}

			clientIP := data.Sessions[i].Creator_ip
			clientID := data.Sessions[i].Client_id
			status := data.Sessions[i].Status
			billing := data.Sessions[i].Billing_type
			a_comment := strings.ReplaceAll(data.Sessions[i].Abort_comment, ";", ":")
			score := data.Sessions[i].Score
			score_r := data.Sessions[i].Score_reason
			// записываем данные по сессии
			writer.Write([]string{idsession, serverName, game, clientIP, city, region, asn, clientID, status, sessionOn, sessionOff, sessionDur, sessionDurUnix, a_comment, score, score_r, comment, billing})
			i++
			fmt.Println(serverName, " - ", i)
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

// offline инфо по IP
func getASNRecord(mmdbCity, mmdbASN string, ip net.IP) (*CityRecord, *ASNRecord, error) {
	dbASN, err := maxminddb.Open(mmdbASN)
	if err != nil {
		return nil, nil, err
	}
	defer dbASN.Close()

	var recordASN ASNRecord
	err = dbASN.Lookup(ip, &recordASN)
	if err != nil {
		return nil, nil, err
	}

	db, err := maxminddb.Open(mmdbCity)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	var recordCity CityRecord
	err = db.Lookup(ip, &recordCity)
	if err != nil {
		return nil, nil, err
	}

	var Subdivision CityRecord
	err = db.Lookup(ip, &Subdivision)
	if err != nil {
		return nil, nil, err
	}
	return &recordCity, &recordASN, err
}

// полученные данных из оффлайн базы
func offlineDBip(ip string) (city, region, asn string) {
	cityRecord, asnRecord, err := getASNRecord(mmdbCity, mmdbASN, net.ParseIP(ip))
	if err != nil {
		log.Println(err)
	}

	asn = asnRecord.AutonomousSystemOrganization // провайдер клиента
	if err != nil {
		log.Println(err, getLine())
		asn = ""
	}

	if val, ok := cityRecord.City.Names["ru"]; ok { // город клиента
		city = val
		if err != nil {
			log.Println(err, getLine())
			city = ""
		}
	} else {
		if val, ok := cityRecord.City.Names["en"]; ok {
			city = val
			if err != nil {
				log.Println(err, getLine())
				city = ""
			}
		}
	}

	if len(cityRecord.Subdivision) > 0 {
		if val, ok := cityRecord.Subdivision[0].Names["ru"]; ok { // регион клиента
			region = val
			if err != nil {
				log.Println(err, getLine())
				region = ""
			}
		} else {
			if val, ok := cityRecord.Subdivision[0].Names["en"]; ok {
				region = val
				if err != nil {
					log.Println(err, getLine())
					region = ""
				}
			}
		}
	}

	return city, region, asn
}

// получаем данные из реестра
func regGet(regFolder, keys string) string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, regFolder, registry.QUERY_VALUE)
	if err != nil {
		log.Printf("Failed to open registry key: %v. %s\n", err, getLine())
	}
	defer key.Close()

	value, _, err := key.GetStringValue(keys)
	if err != nil {
		log.Printf("Failed to read last_server value: %v. %s\n", err, getLine())
	}

	return value
}

// получение строки кода где возникла ошибка
func getLine() string {
	_, _, line, _ := runtime.Caller(1)
	lineErr := fmt.Sprintf("\nОшибка в строке: %d", line)
	return lineErr
}

func updateGeoLite(mmdbASN, mmdbCity string) {
	asnURL := "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb"
	cityURL := "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb"
	z := downloadAndReplaceFileIfNeeded(asnURL, mmdbASN)
	z += downloadAndReplaceFileIfNeeded(cityURL, mmdbCity)
	if z > 0 {
		log.Println("[INFO] Перезапуск приложения")
		restart()
	}
}

func downloadAndReplaceFileIfNeeded(url, filename string) int8 {
	var z int8 = 0
	time.Sleep(2 * time.Second)
	resp, err := http.Get("https://api.github.com/repos/P3TERX/GeoLite.mmdb/releases/latest")
	if err != nil {
		log.Println("[ERROR] Ошибка: ", err, getLine())
		restart()
	}
	defer resp.Body.Close()

	var release Release
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		log.Println("[ERROR] Ошибка: ", err, getLine())
	}

	fileInfo, err := os.Stat(filename)
	if err != nil {
		log.Println("[ERROR] Ошибка получения информации по файлу: ", err, getLine())
	}
	fileModTime := fileInfo.ModTime()

	if fileModTime.Before(release.PublishedAt) {
		// Отправка GET-запроса для загрузки файла
		resp, err := http.Get(url)
		if err != nil {
			log.Println("[ERROR] Ошибка отправки запроса: ", err, getLine())
		}
		defer resp.Body.Close()

		// Создание нового файла и копирование данных из тела ответа
		out, err := os.Create(filename)
		if err != nil {
			log.Println("[ERROR] Ошибка: ", err, getLine())
		}
		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			log.Println("[ERROR] Ошибка замены файлов: ", err, getLine())
		} else {
			log.Printf("[INFO] Файл %s обновлен\n", filename)
			z++
		}
	} else {
		log.Printf("[INFO] Файл %s уже обновлен\n", filename)
		z = 0
	}
	return z
}

// перезапуск приложения
func restart() {
	// Получаем путь к текущему исполняемому файлу
	execPath, err := os.Executable()
	if err != nil {
		log.Println(err, getLine())
	}

	// Запускаем новый экземпляр приложения с помощью os/exec
	cmd := exec.Command(execPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Запускаем новый процесс и не ждем его завершения
	err = cmd.Start()
	if err != nil {
		log.Println(err, getLine())
	}

	// Завершаем текущий процесс
	os.Exit(0)
}

// func ipInf(ip string) (string, string) {
// 	apiURL := fmt.Sprintf("https://ipinfo.io/%s/json", ip)

// 	resp, err := http.Get(apiURL)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	defer resp.Body.Close()

// 	var ipInfo IPInfoResponse
// 	err = json.NewDecoder(resp.Body).Decode(&ipInfo)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	return ipInfo.City, ipInfo.ISP
// }
