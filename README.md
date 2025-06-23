# BitTorrent

<p align="center">
  <a href="" rel="noopener">
 <img src="https://i.ibb.co/vtd7tPG/BIT-TORRENT-6-23-2025.png" alt="Project Logo"></a>
</p>

## 📖 Версия / Version
- [🇷🇺 RU](#RU)
- [🇺🇸 ENG](#ENG)

---

## 🇷🇺 RU <a name="RU"></a>

## 📝 Содержимое

- [О проекте](#О-проекте)
- [Принцип работы BitTorrent](#Принцип-работы-BitTorrent)
- [Структура проекта](#Структура-проекта)
- [Требования и установка](#Требования-и-установка)
- [Примеры использования](#Примеры-использования)
- [Тестирование и отладка](#Тестирование-и-отладка)
- [Зависимости](#Зависимости)
- [Возможные улучшения](#Возможные-улучшения)
- [Заключение](#Заключение)
- [Авторы](#Авторы)

---

## 🧐 О проекте <a name="О-проекте"></a>

Проект **BitTorrent** — это реализация клиента для протокола BitTorrent, написанного на языке программирования Go. BitTorrent — это пиринговый (P2P) протокол, который позволяет пользователям эффективно обмениваться файлами через Интернет. Основная идея протокола заключается в том, чтобы разбивать файлы на небольшие фрагменты `(pieces)`, и распределять их между участниками сети (пирами), что делает загрузку быстрой и устойчивой даже при большом количестве пользователей.

Этот проект создан с образовательной целью и демонстрирует ключевые аспекты работы BitTorrent-клиента:

- Чтение и анализ торрент-файлов.
- Взаимодействие с трекерами для поиска пиров.
- Соединение с пирами и загрузка файлов кусками.
- Сборка и сохранение файлов из полученных фрагментов.

Проект будет полезен разработчикам, изучающим сетевые протоколы, многопоточность в Go, а также всем, кто хочет глубже разобраться в том, как работает BitTorrent.

---

## 🔄 Принцип работы BitTorrent <a name="Принцип-работы-BitTorrent"></a>

BitTorrent — это децентрализованный протокол, который позволяет пользователям обмениваться данными напрямую, без необходимости мощных центральных серверов. Основными элементами протокола являются небольшие куски данных, на которые разбивается файл. Давайте разберем, как это работает.

### Основные этапы работы клиента

1. **Чтение торрент-файла**\
   Торрент-файл (`.torrent`) — это компактный файл с метаданными, который содержит:

   - Адрес трекера (или нескольких трекеров).
   - Хэш-суммы всех фрагментов данных (SHA-1), чтобы проверять их целостность.
   - Информацию о файлах (имена, размеры, структура).
   - Размер каждого куска (обычно от 256 КБ до 4 МБ).\
   Клиент использует библиотеку `bencode-go` для парсинга торрент-файла и вычисляет `InfoHash` — уникальный идентификатор раздачи, который используется для связи с трекером и пирами.

2. **Запрос к трекеру**\
   Клиент отправляет запрос к трекеру (по HTTP или UDP), передавая:

   - `InfoHash` — идентификатор раздачи.
   - `PeerID` — уникальный идентификатор клиента.
   - Порт для входящих соединений (например, 6881).\
   Трекер отвечает списком пиров — других участников сети, у которых есть нужные фрагменты данных.\
   Пример запроса:

   ```
   GET /announce?info_hash=xxx&peer_id=yyy&port=6881&uploaded=0&downloaded=0&left=1000000
   ```

    <p align="center">
      <a href="" rel="noopener">
     <img src="https://i.ibb.co/pBLQ6JNZ/Chat-GPT-Image-Jun-23-2025-05-12-55-PM.png" alt="Запрос к трекеру"></a>
    </p>

3. **Рукопожатие с пирами**\
   Клиент устанавливает TCP-соединение с пирами из списка и отправляет сообщение-рукопожатие (handshake):

   - Длина имени протокола (1 байт, обычно 19).
   - Имя протокола (`BitTorrent protocol`).
   - Зарезервированные байты (8 байт).
   - `InfoHash` (20 байт).
   - `PeerID` (20 байт).\
   Пример в шестнадцатеричном формате:

   ```
   13:BitTorrent protocol00000000<20-byte-infohash><20-byte-peerid>
   ```

   После успешного рукопожатия пир подтверждает, что готов обмениваться данными.

4. **Загрузка файлов**\
   Файлы в BitTorrent не загружаются целиком — они разбиваются на **фрагменты (pieces)** фиксированного размера (например, 256 КБ). Каждый фрагмент, в свою очередь, делится на **блоки** (обычно 16 КБ), которые запрашиваются у пиров.\
   Процесс обмена данными включает следующие сообщения:

   - `Choke`/`Unchoke`: Пир сообщает, готов ли он делиться данными.
   - `Interested`: Клиент указывает, какие фрагменты ему нужны.
   - `Request`: Запрос конкретного блока фрагмента.
   - `Piece`: Передача блока от пира клиенту.\
   Клиент одновременно запрашивает разные фрагменты данных у разных пиров, что ускоряет загрузку.

    <p align="center">
      <a href="" rel="noopener">
     <img src="https://i.postimg.cc/Gp0T01WH/p2p-Photoroom.png" alt="P2P"></a>
    </p>

   После получения всех блоков фрагмента клиент проверяет её хэш-сумму (SHA-1), чтобы убедиться, что данные не повреждены.

5. **Сборка и сохранение файлов**\
   Когда все фрагменты загружены, клиент собирает их в исходный файл (или файлы) и сохраняет на диск. Если это многофайловая раздача, каждый кусок данных может содержать части разных файлов, и клиент распределяет данные по соответствующим местам.

### Подробно о фрагментах (pieces)

- **Что такое фрагмент в контексте BitTorrent?**\
  Это основная единица данных в BitTorrent. Например, если файл размером 10 МБ разбит на фрагменты по 256 КБ, то получится 40 фрагментов. Каждый кусок имеет уникальную хэш-сумму, указанную в торрент-файле, что позволяет проверять её целостность.

- **Размер фрагментов**\
  Обычно варьируется от 256 КБ до 4 МБ. Меньшие фрагменты ускоряют начальную загрузку, но увеличивают объем метаданных. Большие куски данных уменьшают количество хэшей, но требуют больше времени на загрузку каждой.

- **Блоки внутри фрагментов**\
  Фрагменты делятся на блоки (обычно 16 КБ), которые запрашиваются у пиров по одному. Это позволяет загружать фрагмент частями от разных источников одновременно.

- **Пример**\
  Если фрагмент размером 256 КБ состоит из 16 блоков по 16 КБ, клиент может запросить блоки 0–7 у одного пира, а 8–15 у другого, ускоряя процесс.

    <p align="center">
      <a href="" rel="noopener">
     <img src="https://i.postimg.cc/x1LTzNZ1/Editor-Mermaid-Chart-2025-06-23-143832-Photoroom.png" alt="Строение фрагмента данных"></a>
    </p>

### Технические детали

- **Транспорт**: TCP для пиров, UDP для некоторых трекеров.
- **Хэширование**: SHA-1 для проверки фрагментов.
- **Параллельность**: Клиент может загружать до 5–10 фрагментов одновременно, в зависимости от настроек.

---

## 🏗️ Структура проекта <a name="Структура-проекта"></a>

Проект организован модульно для удобства разработки и поддержки. Вот подробное описание файлов:

- `torrent.go`\
  Основные структуры данных:

  - `TorrentFile`: Хранит метаданные торрент-файла (список фрагментов, хэши, файлы).
  - `Peer`: Данные о пире (IP, порт).
  - `Piece`: Структура для работы с фрагментами (индекс, хэш, данные).

- `p2p.go`\
  Логика взаимодействия с пирами:

  - Установка TCP-соединений.
  - Обработка рукопожатий.
  - Поблочная загрузка фрагментов с использованием сообщений `Request` и `Piece`.

- `tracker.go`\
  Коммуникация с трекерами:

  - Поддержка HTTP- и UDP-запросов.
  - Парсинг ответа с информацией о пирах.

- `utils.go`\
  Вспомогательные функции:

  - Генерация `PeerID`.
  - Проверка хэшей фрагментов.
  - Логирование прогресса загрузки.

- `parse.go`\
  Парсинг торрент-файлов:

  - Декодирование `bencode`.
  - Извлечение списка фрагментов и их хэшей.

- `main.go`\
  Точка входа:

  - Парсинг аргументов (путь к `.torrent`, директория сохранения).
  - Координация работы модулей (чтение, загрузка фрагментов, сохранение).

---

## ⚙️ Требования и установка <a name="Требования-и-установка"></a>

### Требования

- **Go**: Версия 1.21+.
- **ОС**: Linux, macOS, Windows.
- **Сеть**: Доступ к трекерам и пирам.

### Установка

1. **Клонирование репозитория**:

   ```bash
   git clone git@github.com:lvbealr/BitTorrent.git
   cd BitTorrent
   ```
2. **Проверка Go**:

   ```bash
   go version
   ```
3. **Установка зависимостей**:

   ```bash
   go mod tidy
   ```
4. **Компиляция**:

   ```bash
   go build
   ```
5. **Запуск**:

   ```bash
   ./bittorrent <путь-к-торренту> <путь-для-сохранения>
   ```

---

## 🕹️ Примеры использования <a name="Примеры-использования"></a>

### Загрузка торрента

```bash
./BitTorrent <торрент-файл> <выходной-путь>
# ./BitTorrent archlinux-2025.06.01-x86_64.iso.torrent arch-dir
# [archlinux-2025.06.01-x86_64.iso]	[»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»----------] (78.32/100%) [3.31 MB/s]
```

---

## 🛠️ Отладка <a name="Тестирование-и-отладка"></a>

- **Логи**:
    Доступны в файле `torrent.log`, лежащем в корневой директории.
- **Тест пиров**:

  ```bash
  nc -l 6881
  ```

---

## 📦 Зависимости <a name="Зависимости"></a>

- `github.com/jackpal/bencode-go`: Парсинг торрент-файлов.
- `github.com/cespare/xxhash/v2`: Быстрое хэширование.\
  Установка:

```bash
go get github.com/jackpal/bencode-go
go get github.com/cespare/xxhash/v2
```

---

## 🔮 Возможные улучшения <a name="Возможные-улучшения"></a>

- **DHT**: Поиск пиров без трекеров.
- **Магнит-ссылки**: Загрузка по `magnet`.

---

## 🏁 Заключение <a name="Заключение"></a>

Этот проект — базовый BitTorrent-клиент, демонстрирующий работу с торрентами и пирами.

---

## ✍ Авторы <a name="Авторы"></a>

- @lvbealr — идея и разработка.

---

## 🇺🇸 ENG <a name="ENG"></a>

## 📝 Table of Contents

- [About](#About)
- [How BitTorrent Works](#How-BitTorrent-Works)
- [Project Structure](#Project-Structure)
- [Requirements and Installation](#Requirements-and-Installation)
- [Example Usage](#Example-Usage)
- [Testing and Debugging](#Testing-and-Debugging)
- [Dependencies](#Dependencies)
- [Possible Improvements](#Possible-Improvements)
- [Conclusion](#Conclusion)
- [Authors](#Authors)

---

## 🧐 About <a name="About"></a>

The **BitTorrent** project is an implementation of a BitTorrent client written in the Go programming language. BitTorrent is a peer-to-peer (P2P) protocol that enables efficient file sharing over the internet. The core idea is to split files into small **pieces** and distribute them among network participants (peers), ensuring fast and reliable downloads even with a large number of users.

This project serves an educational purpose and highlights key aspects of a BitTorrent client:

- Reading and parsing torrent files.
- Interacting with trackers to locate peers.
- Connecting to peers and downloading files in pieces.
- Assembling and saving files from the downloaded pieces.

It is a valuable resource for developers exploring network protocols, concurrency in Go, or anyone interested in understanding how BitTorrent operates.

---

## 🔄 How BitTorrent Works <a name="How-BitTorrent-Works"></a>

BitTorrent is a decentralized protocol that allows users to exchange data directly, eliminating the need for powerful central servers. Its primary components are small data **pieces** into which files are divided. Let’s break down how it works.

### Key Stages of Client Operation

1. **Reading the Torrent File**  
   A `.torrent` file is a compact metadata file containing:  
   - Tracker address(es).  
   - SHA-1 hashes of all data pieces for integrity verification.  
   - File details (names, sizes, structure).  
   - Piece size (typically 256 KB to 4 MB).  
   The client uses the `bencode-go` library to parse the torrent file and calculates the `InfoHash`—a unique identifier for the torrent, used to communicate with trackers and peers.

2. **Tracker Request**  
   The client sends a request to the tracker (via HTTP or UDP) with:  
   - `InfoHash`—torrent identifier.  
   - `PeerID`—unique client identifier.  
   - Listening port (e.g., 6881).  
   The tracker responds with a list of peers who have the required data pieces.  
   Example request:  
   ```
   GET /announce?info_hash=xxx&peer_id=yyy&port=6881&uploaded=0&downloaded=0&left=1000000
   ```  

   <p align="center">
      <a href="" rel="noopener">
     <img src="https://i.ibb.co/pBLQ6JNZ/Chat-GPT-Image-Jun-23-2025-05-12-55-PM.png" alt="Tracker Request"></a>
    </p>

3. **Peer Handshake**  
   The client establishes a TCP connection with peers from the list and sends a handshake message:  
   - Protocol name length (1 byte, typically 19).  
   - Protocol name (`BitTorrent protocol`).  
   - Reserved bytes (8 bytes).  
   - `InfoHash` (20 bytes).  
   - `PeerID` (20 bytes).  
   Example in hexadecimal:  
   ```
   13:BitTorrent protocol00000000<20-byte-infohash><20-byte-peerid>
   ```  
   Upon a successful handshake, the peer confirms readiness to exchange data.

4. **File Download**  
   Files in BitTorrent are not downloaded whole—they are split into **pieces** of fixed size (e.g., 256 KB). Each piece is further divided into **blocks** (typically 16 KB), which are requested from peers. The data exchange involves:  
   - `Choke`/`Unchoke`: Peer indicates willingness to share data.  
   - `Interested`: Client specifies needed pieces.  
   - `Request`: Request for a specific block.  
   - `Piece`: Data block sent from peer to client.  
   The client requests different pieces from multiple peers simultaneously, speeding up the download.  

    <p align="center">
      <a href="" rel="noopener">
     <img src="https://i.postimg.cc/Gp0T01WH/p2p-Photoroom.png" alt="P2P"></a>
    </p>

   After receiving all blocks of a piece, the client verifies its SHA-1 hash to ensure data integrity.

5. **File Assembly and Saving**  
   Once all pieces are downloaded, the client assembles them into the original file(s) and saves them to disk. In multi-file torrents, each piece may contain parts of different files, and the client allocates the data accordingly.

### Deep Dive into Pieces

- **What is a Piece in BitTorrent?**  
  A piece is the fundamental data unit in BitTorrent. For instance, a 10 MB file split into 256 KB pieces yields 40 pieces, each with a unique hash listed in the torrent file for integrity checks.

- **Piece Size**  
  Typically ranges from 256 KB to 4 MB. Smaller pieces accelerate initial downloads but increase metadata overhead. Larger pieces reduce the number of hashes but take longer to download.

- **Blocks within Pieces**  
  Pieces are subdivided into blocks (usually 16 KB), requested individually from peers, enabling parallel downloads from multiple sources.

- **Example**  
  A 256 KB piece with 16 blocks of 16 KB each allows the client to request blocks 0–7 from one peer and 8–15 from another, optimizing speed.  

    <p align="center">
      <a href="" rel="noopener">
     <img src="https://i.ibb.co/WNJmXd8M/Editor-Mermaid-Chart-2025-06-23-144214-Photoroom.png" alt="Piece structure"></a>
    </p>

### Technical Details

- **Transport**: TCP for peers, UDP for some trackers.  
- **Hashing**: SHA-1 for piece verification.  
- **Concurrency**: The client can download 5–10 pieces simultaneously, depending on configuration.

---

## 🏗️ Project Structure <a name="Project-Structure"></a>

The project is modularly organized for ease of development and maintenance. Here’s a detailed file breakdown:

- **`torrent.go`**  
  Core data structures:  
  - `TorrentFile`: Stores torrent metadata (piece list, hashes, files).  
  - `Peer`: Peer information (IP, port).  
  - `Piece`: Structure for handling pieces (index, hash, data).

- **`p2p.go`**  
  Peer interaction logic:  
  - TCP connection setup.  
  - Handshake processing.  
  - Block-by-block piece downloading using `Request` and `Piece` messages.

- **`tracker.go`**  
  Tracker communication:  
  - Supports HTTP and UDP requests.  
  - Parses peer information from responses.

- **`utils.go`**  
  Utility functions:  
  - `PeerID` generation.  
  - Piece hash verification.  
  - Download progress logging.

- **`parse.go`**  
  Torrent file parsing:  
  - `bencode` decoding.  
  - Extraction of piece lists and hashes.

- **`main.go`**  
  Entry point:  
  - Argument parsing (`.torrent` path, save directory).  
  - Coordination of modules (reading, downloading pieces, saving).

---

## ⚙️ Requirements and Installation <a name="Requirements-and-Installation"></a>

### Requirements

- **Go**: Version 1.21+.  
- **OS**: Linux, macOS, Windows.  
- **Network**: Access to trackers and peers.

### Installation

1. **Clone the Repository**:  
   ```bash
   git clone git@github.com:lvbealr/BitTorrent.git
   cd BitTorrent
   ```

2. **Verify Go Installation**:  
   ```bash
   go version
   ```

3. **Install Dependencies**:  
   ```bash
   go mod tidy
   ```

4. **Build the Project**:  
   ```bash
   go build
   ```

5. **Run the Client**:  
   ```bash
   ./bittorrent <torrent-path> <save-path>
   ```

---

## 🕹️ Example Usage <a name="Example-Usage"></a>

### Downloading a Torrent

```bash
./BitTorrent <torrent-file> <output-path>
# Example:
# ./BitTorrent archlinux-2025.06.01-x86_64.iso.torrent arch-dir
# [archlinux-2025.06.01-x86_64.iso]	[»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»»----------] (78.32/100%) [3.31 MB/s]
```

---

## 🛠️ Testing and Debugging <a name="Testing-and-Debugging"></a>

- **Logs**:  
  Available in `torrent.log` in the root directory.

- **Peer Testing**:  
  ```bash
  nc -l 6881
  ```

---

## 📦 Dependencies <a name="Dependencies"></a>

- **`github.com/jackpal/bencode-go`**: Torrent file parsing.  
- **`github.com/cespare/xxhash/v2`**: Fast hashing.  

**Installation**:  
```bash
go get github.com/jackpal/bencode-go
go get github.com/cespare/xxhash/v2
```

---

## 🔮 Possible Improvements <a name="Possible-Improvements"></a>

- **DHT**: Peer discovery without trackers.  
- **Magnet Links**: Support for `magnet` URI downloads.

---

## 🏁 Conclusion <a name="Conclusion"></a>

This project provides a foundational BitTorrent client, illustrating torrent handling and peer interactions.

---

## ✍ Authors <a name="Authors"></a>

- **@lvbealr** — Concept and development.

---