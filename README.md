# Drova-Session-INFO
Позволяет владельцам станций выгружать последние игровые сессии используя API сайта Drova.
Для работы потребуются файл GeoLite2-ASN.mmdb и GeoLite2-City.mmdb. Положит в папку с программой

Запуск

1. Скопируйте все файлы на свой локальный компьютер и распакуйте.
2. Скачайте базы данных IP адресов GeoLite2 (файлы называется GeoLite2-City.mmdb и GeoLite2-ASN.mmdb) в папку с программой. Если файлов не будет, инфа по IP будет пустая(город, область, провайдер)
3. Установить Golang https://go.dev/
4. Запускаем copilate.bat, получаем исполняемый файл
5. Заменяем в config.txt значение token на свой токен. Получить его можно на странице станций. Так же можно запустить на действующей станции, тогда файл конфига можно удалить, токен будет получен со станции
7. Запускаем исполняемый файл, по окончании работы программа в папке появится файл csv.
8. Если возникнут ошибки, они будут записаны в файл errors.log.

Для корректного отображения кириллицы в Excel, файл с сессиями необходимо импортировать (Вкладка Данные->Из текстового/CSV-файла). Кодировка UTF-8, разделитель запятая
