# «Студия автора» на Windows из приватного Docker Hub

Эта папка — самодостаточный пакет для друга: ему не нужны исходники проекта.
Нужны только Docker Desktop, собственный Docker Hub аккаунт с выданным доступом и
три файла из этой папки: `compose.yaml`, `.env.example` и этот `README.md`.

Образ приложения: `example/writers-studio:0.2.0-amd64`. Суффикс `amd64`
означает обычные компьютеры Windows с процессорами Intel/AMD. Не заменяйте его
на `latest`: конкретный тег делает установку воспроизводимой.

## 1. Выдать другу доступ в Docker Hub

Docker Hub Personal разрешает private repository, но **не разрешает добавить
collaborator**. Установку для одного друга можно организовать одним из двух
способов.

### Вариант A — Docker Pro: доступ на аккаунт друга

1. Попросите друга создать свой Docker Hub аккаунт и прислать **Docker ID**
   (не e-mail и не пароль).
2. Подключите Docker Pro к аккаунту `example-owner`.
3. Откройте <https://hub.docker.com/> под аккаунтом `example-owner`.
4. Откройте **My Hub → Repositories → writers-studio → Collaborators**.
5. Добавьте Docker ID друга и дождитесь, пока он примет приглашение.

Репозиторий уже private. Не передавайте другу пароль от своего Docker Hub и не
меняйте repository visibility на public. В персональном Docker Hub collaborator
получает и pull, и push; Docker Pro поддерживает до одного collaborator для
private personal repository.

### Вариант B — без смены тарифа: отдельный временный read-only token

Это удобный вариант для одного доверенного друга. В Docker Hub под
`example-owner` откройте **Account settings → Personal access tokens → Generate
new token**, назовите его, например, `writers-studio-friend`, задайте короткий
срок действия и выберите доступ **Read**. Скопируйте токен: второй раз Docker
Hub его не покажет.

Передайте токен через защищённый канал (не в общем чате). Друг выполняет:

```powershell
docker login --username example-owner
```

В качестве пароля он вставляет **токен**, а не пароль от вашего аккаунта.
Такой token подходит только для скачивания, его можно отдельно отозвать в том
же разделе. Помните: это не персональная привязка к Docker ID друга; любой, кто
получит token, сможет читать private repositories, доступные вашему аккаунту.
Для долгосрочного доступа предпочтителен Вариант A.

После отзыва collaborator новые pull станут недоступны, но уже скачанный образ
на его компьютере не исчезнет автоматически.

## 2. Передать пакет без секретов

Передайте другу только содержимое этой папки, например архивом:

```text
writers-studio/
  compose.yaml
  .env.example
  README.md
```

Не передавайте свой `.env`: в нём могут быть ключи Deepgram, OpenRouter или LLM.
Они не включены в Docker image и не нужны для первого запуска.

## 3. Подготовить Windows-компьютер друга

1. Установить и запустить Docker Desktop for Windows.
2. В настройках Docker Desktop должен быть включён режим **Linux containers**;
   WSL 2 backend — рекомендуемый вариант.
3. Открыть PowerShell в папке `writers-studio` и проверить установку:

   ```powershell
   docker version
   docker compose version
   ```

4. Войти в Docker Hub. В Варианте A — под Docker ID друга; в Варианте B — под
   пользователем `example-owner` и выданным read-only token:

   ```powershell
   docker login
   ```

   Docker попросит открыть браузер и подтвердить одноразовый код. Это нормально.
   При Варианте A нельзя выполнять вход под аккаунтом издателя.

## 4. Создать локальный `.env`

Docker Compose читает файл, переданный через `--env-file .env`. В `compose.yaml`
эти значения подставляются в конфигурацию PostgreSQL, MinIO и приложения, но
сам `.env` **не попадает в image** и не отправляется в Docker Hub.

Создайте файл и откройте его:

```powershell
Copy-Item .env.example .env
notepad .env
```

Оставьте эту строку без изменений:

```dotenv
WRITERS_STUDIO_IMAGE=example/writers-studio:0.2.0-amd64
```

Для `POSTGRES_PASSWORD` и `OBJECT_STORAGE_SECRET_KEY` вставьте два разных
случайных значения. Они должны состоять только из букв и цифр (так пароль
безопасно работает в URL подключения к PostgreSQL). В PowerShell можно получить
два подходящих значения так:

```powershell
([guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')).Substring(0,48)
```

В `.env` обязательно замените оба значения-заглушки:

```dotenv
POSTGRES_PASSWORD=вставьте_первое_случайное_значение
OBJECT_STORAGE_SECRET_KEY=вставьте_второе_случайное_значение
```

Остальное можно оставить как есть для первого запуска. Без ключей Deepgram и
OpenRouter приложение откроется, но распознавание речи будет честно выключено.
Для работы диктовки друг должен вставить **свои** ключи:

```dotenv
DEEPGRAM_API_KEY=...
OPENROUTER_API_KEY=...
```

Если он будет сохранять ключи LLM-провайдеров внутри приложения, также создайте
и сохраните отдельно ключ шифрования (его нельзя менять после появления таких
данных):

```powershell
$bytes = New-Object byte[] 32
[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
[Convert]::ToBase64String($bytes)
```

Вставьте результат в `LLM_CREDENTIAL_ENCRYPTION_KEY=`. Если LLM пока не нужен,
это поле оставляют пустым.

## 5. Первый запуск

Из папки с файлами выполните:

```powershell
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
docker compose --env-file .env -f compose.yaml ps
curl.exe -fsS http://localhost:8080/api/health
```

В `ps` все три сервиса (`app`, `db`, `storage`) должны быть `running` или
`healthy`. Затем открыть <http://localhost:8080> и создать локальный аккаунт
приложения.

Приложение опубликовано только на `127.0.0.1:8080`: его не видно другим
устройствам в домашней сети. На `localhost` браузер допускает доступ к
микрофону, поэтому диктовка в браузере работает без отдельного HTTPS-прокси.

## 6. Если что-то не запустилось

```powershell
docker compose --env-file .env -f compose.yaml logs --tail=150 app
docker compose --env-file .env -f compose.yaml logs --tail=150 db storage
```

- `pull access denied` / `requested access to the resource is denied` — друг
  не принял invitation (Вариант A), вошёл не под тем Docker ID или token был
  отозван/истёк (Вариант B). Нужно сделать `docker logout`, затем `docker login`
  и повторить `pull`.
- Порт `8080` занят — в `.env` задайте, например, `APP_PORT=8081`, затем
  выполните `up -d` и откройте <http://localhost:8081>.
- Не запускайте `docker compose down -v`: эта команда удалит PostgreSQL и
  MinIO volumes, а вместе с ними рукописи, аудио и настройки приложения.

## 7. Обновление и остановка

Для нового релиза издатель выпускает новый явный тег, например
`0.2.1-amd64`. Друг меняет только `WRITERS_STUDIO_IMAGE` в `.env`, затем
выполняет:

```powershell
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
```

Остановка без удаления данных:

```powershell
docker compose --env-file .env -f compose.yaml down
```

Именованные Docker volumes сохраняют данные при `down`, `pull` и обновлении
контейнеров.
