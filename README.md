# Drova-Session-INFO
Позволяет владельцам станций выгружать последние игровые сессии используя API сайта Drova.
Для работы потребуются файл GeoLite2-ASN.mmdb и GeoLite2-City.mmdb. Положит в папку с программой

Запуск

1. Скопируйте все файлы на свой локальный компьютер и распакуйте.
2. Скачайте базы данных IP адресов GeoLite2 (файлы называется GeoLite2-City.mmdb и GeoLite2-ASN.mmdb) в папку с программой.
3. Установить Golang https://go.dev/
4. Открываем коммандную строку и переходим в распакованную папку.
5. Выполняем команду go build -o drova_session_info.exe main.go.
6. Заменяем в config.txt YOU_TOKEN на свой. Получить его можно на странице станций.
7. Запускаем исполняемый файл drova_session_info.exe, по окончании работы программа в папке появится файл csv.
8. Если возникнут ошибки, они будут записаны в файл errors.log.
