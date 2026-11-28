# EPA — Everyone's Package Adapter

**Your commands. Any distribution.**

EPA는 ROKFOSS가 Go로 개발하는 실험적 리눅스 패키지 관리자 어댑터입니다. 배포판마다 다른 명령과 터미널 출력을 통합하되, 실제 작업은 운영체제에 설치된 패키지 관리자에 맡깁니다. 패키지 데이터베이스, 저장소 설정, 패키지 형식, 서명 검증, 의존성 해결기를 대체하지 않습니다.

패키지 이름은 자동 변환하지 않습니다. Debian의 `apache2`, RHEL의 `httpd`, Arch의 `apache`처럼 현재 배포판에서 제공하는 이름을 입력하세요.

## 설치 · 업데이트

Go나 소스 빌드 없이 최신 정식 릴리스의 Linux 바이너리를 설치합니다. 터미널에서 다음 한 줄을 실행하세요.

```sh
( p=$(mktemp) && trap 'rm -f -- "$p"' EXIT && curl -qfsSL --proto '=https' --proto-redir '=https' -o "$p" https://github.com/KRFOSS/epa/releases/latest/download/install.sh && sh "$p" )
```

설치기는 아키텍처를 자동으로 감지하고 GitHub 릴리스의 SHA-256 목록과 바이너리를 비교한 뒤 `epa`를 설치합니다. 자체 업데이트를 지원하지 않는 기존 버전 또는 기본 경로 밖의 설치를 갱신할 때는 같은 명령을 사용합니다. 자동으로 `sudo`를 호출하지 않습니다. 일반 사용자는 `~/.local/bin/epa`, root는 `/usr/local/bin/epa`에 설치합니다. 다른 위치는 환경 변수 `EPA_BIN_DIR`에 절대 경로를 지정하세요. 설치 위치가 `PATH`에 없다면 경로를 추가하거나 `~/.local/bin/epa --version`처럼 절대 경로로 실행하세요.

- **배포 대상:** x86-64, x86 32비트, ARMv5/v6/v7, ARM64, RISC-V 64비트, LoongArch 64비트, MIPS 32/64비트(양쪽 바이트 순서), PowerPC 64비트(양쪽 바이트 순서), IBM Z (`s390x`). Go가 지원하지 않는 CPU·커널 조합은 포함하지 않습니다. PowerPC는 POWER8 이상을 대상으로 합니다.
- **패키지 관리자:** 바이너리 설치 가능 여부와는 별개로, 패키지 조작은 APT·DNF/DNF5·Pacman·APK가 있는 지원 배포판에서만 가능합니다.
- **수동 설치:** [GitHub 릴리스](https://github.com/KRFOSS/epa/releases)에서 자신의 아키텍처에 맞는 `epa-linux-*` 파일과 `SHA256SUMS`를 내려받아 검증한 뒤, 원하는 경로에 `epa`로 설치할 수도 있습니다. 체크섬은 다운로드 손상을 탐지하며 별도 릴리스 서명을 대신하지는 않습니다.

### 자체 업데이트

일반 사용자 계정의 기본 경로(`~/.local/bin/epa`) 또는 root 계정의 기본 경로(`/usr/local/bin/epa`)에 안전하게 설치한 정식 릴리스는 대화형 패키지 갱신·업그레이드가 성공한 뒤 EPA의 새 정식 릴리스도 자동으로 확인합니다. 최신 버전이 현재 버전보다 높을 때만 해당 버전에 고정된 SHA-256 목록과 현재 실행 파일의 아키텍처에 맞는 바이너리를 내려받습니다. 검증과 동기화를 마친 임시 파일을 같은 디렉터리에서 원자적으로 교체하므로 새 버전은 다음 실행부터 적용됩니다.

```sh
epa update
epa upgrade -y
epa --style pacman -Syu
epa self-update                  # EPA만 명시적으로 갱신
epa --no-self-update update      # 이번 실행에서만 생략
epa config set self-update off   # 계정별 자동 갱신 끄기
epa config set self-update on    # 다시 켜기 (기본값)
epa config get self-update
```

- 개발 빌드, 드라이런, 비대화형 패키지 작업, JSON 출력, 실패한 패키지 갱신에서는 자동 업데이트를 수행하지 않습니다. 명시적인 `epa self-update`는 자동 갱신 설정과 별개입니다.
- root로 직접 실행하면 `/usr/local/bin/epa`의 자체 업데이트도 지원합니다. 해당 경로와 상위 디렉터리는 모두 root 소유이며 다른 계정이 쓸 수 없어야 합니다. `sudo epa`처럼 직접 sudo로 호출한 실행, 권한이 변경된 프로세스, 배포판 패키지 관리 경로(`/usr/bin/epa` 등), 사용자 지정 경로는 자체 교체 대상이 아닙니다. 심볼릭 링크 및 하드 링크로 된 설치 파일도 교체하지 않습니다. 자체 업데이트는 권한 상승을 요청하지 않습니다.
- GitHub 조회, 다운로드 또는 SHA-256·실행 파일 형식 검증이 실패하면 기존 파일을 보존하고, 성공한 패키지 작업의 종료 상태도 바꾸지 않습니다. 오래된 버전으로 되돌리지 않습니다.
- 릴리스 체크섬은 전송 손상과 잘못된 파일을 검출하기 위한 것으로 별도의 암호학적 서명이나 GitHub 계정 침해 방지를 대신하지는 않습니다.
- 자체 업데이트가 없는 v0.1.0 또는 root 자체 업데이트를 지원하지 않는 v0.1.1~v0.1.2에서 처음 전환할 때는 위 설치 명령을 한 번 더 실행해야 합니다.

### 소스 빌드 (개발용)

Linux와 Go 1.23 이상이 필요합니다.

```sh
CGO_ENABLED=0 go build -trimpath -o epa ./cmd/epa
```

배포는 `main`에 포함된 커밋에 `vX.Y.Z` 형식의 태그를 푸시하거나 GitHub Actions의 `Linux 배포` 워크플로에서 정식 버전과 검증한 `main` 커밋의 전체 SHA를 지정하여 수동 실행하면 시작됩니다. 수동 배포는 빌드와 설치 검증을 통과한 뒤에만 새 태그를 만들며 기존 태그를 덮어쓰지 않습니다. 업로드가 중단되면 동일한 태그와 커밋으로 초안 릴리스를 재시도할 수 있습니다. 정식 게시를 직렬화하고 이전 버전이 최신 정식 릴리스를 대체하지 못하도록 차단합니다. 모든 바이너리, 설치 스크립트, SHA-256 목록의 업로드를 확인한 뒤 초안 릴리스를 정식 게시합니다.

## 사용법

```sh
epa style list
epa style set apt
epa style current
epa update
epa install httpd -y       # RHEL/Fedora 예시
epa --dry-run install httpd -y
epa style set pacman
epa -S nginx
epa --style apt search nginx
epa backend info
epa doctor
```

기본 명령 스타일은 APT입니다. 명령 스타일과 실제 백엔드는 별개입니다. 선택한 스타일은 계정별로 `$XDG_CONFIG_HOME/epa/config.json`(기본값: `~/.config/epa/config.json`)에 저장됩니다. `epa style reset`으로 APT 스타일을 복원할 수 있습니다. 스크립트에서는 명령 앞에 `--style`을 붙여 명령의 의미를 고정하세요.

일반 사용자로 `epa`를 실행하세요. 시스템을 변경하는 작업에서는 네이티브 패키지 관리자만 `sudo`로 실행합니다. EPA 자체를 `sudo`로 실행해야 한다면 `--style`을 지정하세요. `sudo`로 호출한 EPA에서 스타일을 변경할 수는 없지만, root로 직접 로그인하거나 `sudo -i`, `sudo su` 등으로 root 셸에 진입한 경우에는 root 계정의 설정을 사용할 수 있습니다. sudo 환경 변수가 남아 있는 root 셸에서는 원래 사용자의 HOME·XDG 설정을 따르지 않고 root 계정의 홈과 기본 설정 경로를 사용합니다.

## 통합 터미널 출력

EPA 자체 도움말, 안내 및 오류 메시지와 인식한 진행 단계의 기본 언어는 한국어입니다. 계정별로 영어를 선택할 수 있으며, 기존 설정에 언어가 지정되지 않은 경우에도 한국어를 사용합니다.

```sh
epa config get language       # 현재 저장된 언어 코드 조회 (ko 또는 en)
epa config set language en    # 영어로 변경
epa config set language ko    # 한국어로 복원
epa --language en --help      # 이번 실행만 영어
```

명령과 옵션, 자동완성 값, 설정 조회 값 및 JSON 이벤트 필드는 호환성을 위해 기존 형식을 유지합니다. EPA가 인식하는 일반적인 APT 상태·패키지 변경 요약·확인 질문은 선택한 언어로 표시합니다. 인식하지 못한 출력과 입력 프롬프트는 운영체제 및 백엔드의 로캘 설정에 따릅니다. `sudo epa`는 다른 계정의 설정을 읽지 않으므로 필요하면 `--language`를 명시하세요.

대화형 Linux 터미널에서 패키지를 변경하는 작업은 인라인 진행 화면을 사용합니다. APT, DNF/DNF5, Pacman, APK는 같은 렌더러를 사용합니다. 인식한 백엔드 진행 기록을 스피너와 터미널 폭에 맞춘 진행 막대로 표시합니다. APT 다운로드에서 개별 항목 진행률과 전체 진행률을 모두 확인한 경우 두 수치를 구분합니다. 확인할 수 없는 수치는 추정하지 않으며, 명시적인 완료 기록이나 프로세스 종료 상태에 근거해 성공을 표시합니다.

```sh
epa upgrade -y
epa --progress tty install nginx
epa --progress plain upgrade
epa --progress json update
epa --progress native install nginx
epa --progress quiet install nginx
epa config set progress auto
epa config get progress
```

| 모드 | 동작 |
|---|---|
| `auto` | 대화형 변경 작업에는 인라인 화면, 터미널이 아닌 환경의 변경 작업에는 `plain`, 읽기 전용 조회에는 원본 출력을 사용합니다. |
| `tty` | 인라인 애니메이션을 표시합니다. 적합한 터미널이 없으면 `plain`으로 전환합니다. |
| `plain` | 진행 정보를 애니메이션 없이 표시합니다. 터미널이 아니면 시작·종료 안내와 백엔드 출력을 전달합니다. |
| `json` | 표준 출력에 줄 단위 JSON으로 시작·종료 이벤트를 기록하고, 백엔드 출력과 프롬프트는 표준 오류로 전달합니다. |
| `native` | 렌더러 없이 백엔드의 입출력과 종료 상태를 그대로 전달합니다. |
| `quiet` | 대화형 변경 작업에서 EPA 진행 표시와 인식한 백엔드 진행 기록을 생략합니다. 알 수 없는 메시지, 프롬프트, 오류는 보존합니다. 리디렉션된 출력과 조회 결과는 필터링하지 않습니다. |

출력 모드는 계정별로 저장하며, `--progress`로 한 번만 덮어쓸 수 있습니다. Linux 의사 터미널(PTY)을 통해 `sudo`, 패키지 관리자, 패키지 유지보수 스크립트가 입력을 받을 수 있게 합니다. 인식하지 못한 메시지와 프롬프트, 경고와 오류는 그대로 전달합니다. APT의 명시적인 확인 거부와 종료 코드 1을 함께 확인하면 취소로 표시하며, 원래 종료 코드는 유지합니다. 복잡한 터미널 제어 시퀀스가 나오면 해당 작업의 나머지는 원본 출력으로 표시합니다. 렌더러가 확인 질문에 자동으로 응답하거나 비대화형 옵션을 추가하지는 않습니다.

터미널 크기 변경은 백엔드에도 전달합니다. 패키지 목록이 화면에 계속 쌓이지 않도록 현재 관측한 항목 하나를 갱신합니다. `NO_COLOR=1`로 EPA의 색상 출력을 끌 수 있습니다.

현재 진행 표시는 네이티브 CLI 출력 분석에 기반합니다. 배포판, 백엔드 버전, 로캘 또는 출력 형식에 따라 일부 진행 정보는 원본 텍스트로 표시됩니다. 예를 들어 DNF5의 여러 줄 출력은 원본 표시로 전환될 수 있습니다. 특수한 대화형 프로그램이나 터미널에서는 `native`를 사용하세요. `json`은 시작·종료 이벤트 스트림이며 패키지별 진행 이벤트를 제공하지 않습니다.

## 셸 자동완성

공개 설치기는 설치 후 현재 계정에서 사용 가능한 Bash, Zsh 및 Fish의 자동완성을 영구 등록합니다. 새 대화형 셸이나 SSH 재접속부터 적용됩니다. 기존 설정을 덮어쓰지 않고 EPA 전용 블록 또는 Fish 자동완성 파일만 등록하며, 다시 설치해도 중복되지 않습니다.

이미 EPA를 설치했거나 자체 업데이트한 사용자는 다음 명령으로 한 번 등록할 수 있습니다.

```sh
epa completion install       # 설치된 Bash, Zsh, Fish
epa completion install bash  # Bash만 등록
epa completion install zsh   # Zsh만 등록
epa completion install fish  # Fish만 등록
```

Bash는 `~/.bashrc`와 실제 로그인 시작 파일(`~/.bash_profile`, `~/.bash_login`, `~/.profile` 중 우선순위가 가장 높은 파일)을 사용합니다. Zsh는 `~/.zshrc` 또는 환경 변수로 내보낸 `ZDOTDIR`의 `.zshrc`를 사용합니다. Bash·Zsh 로더는 `~/.local/share/epa/completion/`에 설치됩니다. Fish는 `$XDG_CONFIG_HOME/fish/completions/epa.fish`(기본값: `~/.config/fish/completions/epa.fish`)를 자동으로 읽습니다. 기존에 등록된 자동완성은 그대로 유지합니다. Zsh에서 초기화가 필요하면 `compinit -i`로 안전하지 않은 자동완성 경로를 제외합니다. 또한 공유 쓰기 권한이 있거나 심볼릭 링크·하드 링크인 설정 파일은 안전을 위해 자동 수정하지 않습니다. 자동완성 등록에 실패해도 검증된 EPA 바이너리 설치는 유지됩니다.

`sudo`로 설치할 때는 다른 계정의 시작 파일을 변경하지 않습니다. EPA를 사용할 계정에서 `epa completion install`을 실행하세요. 자동 등록을 원하지 않으면 설치 명령의 `sh`에 `EPA_INSTALL_COMPLETION=0`을 지정할 수 있습니다.

```sh
curl -fsSL https://github.com/KRFOSS/epa/releases/latest/download/install.sh | EPA_INSTALL_COMPLETION=0 sh
```

Bash·Zsh 영구 등록을 제거하려면 해당 시작 파일의 `# >>> EPA shell completion >>>`부터 `# <<< EPA shell completion <<<`까지의 블록만 삭제하세요. Fish에서는 EPA가 설치한 `fish/completions/epa.fish`만 삭제하세요. 기존 설정 파일 전체를 삭제하지 마세요.

현재 셸에서 즉시 사용하려면 Bash에서 다음 명령을 실행하세요.

```bash
source <(epa completion bash)
```

Zsh에서는 `compinit`으로 자동완성을 초기화한 뒤 불러오세요.

```zsh
autoload -Uz compinit
compinit
source <(epa completion zsh)
```

Fish에서는 다음 명령으로 현재 셸에 바로 등록할 수 있습니다.

```fish
epa completion fish | source
```

자동완성은 활성 스타일의 명령과 옵션, 실제 백엔드의 패키지 이름을 제안합니다. 설치·검색·정보 조회에는 사용 가능한 패키지를, 제거·재설치에는 설치된 패키지를 제안합니다. 패키지 후보는 기존 로컬 메타데이터로만 조회하며 제한 시간은 2초입니다. 관리자 권한을 요청하지 않으며, 캐시가 없거나 읽을 수 없으면 패키지 후보를 표시하지 않습니다.

## 초기 지원 범위

| 명령 스타일 | 주요 명령 |
|---|---|
| APT | `update`, `upgrade`, `full-upgrade`, `install`, `remove`, `search`, `show`, `list`, `outdated`, `autoremove`, `reinstall`, `clean` |
| DNF | `makecache`, `update`/`upgrade`, `install`, `remove`/`erase`, `search`, `info`, `list`, `check-update`, `autoremove`, `reinstall`, `clean all` |
| Pacman | `-S`, `-Ss`, `-Si`, `-Sy`, `-Syy`, `-Su`, `-Syu`, `-Syyu`, `-R`, `-Rs`, `-Rn`, `-Rns`, `-Q`, `-Qu`, `-Sc` |
| APK | `update`, `add`, `del`, `upgrade`, `search`, `info`, `list --installed`, `list --upgradable`, `fix`, `cache clean` |

백엔드는 APT, DNF(DNF5 포함), Pacman, Alpine Linux의 APK를 대상으로 합니다. `os-release`와 고정된 시스템 실행 파일 경로로 백엔드를 감지합니다. Zypper, 구형 YUM은 아직 지원하지 않습니다.

EPA는 네이티브 명령 전체를 대체하지 않습니다. 알 수 없는 명령과 옵션은 오류로 처리합니다. 버전 지정자, 파일, URL, 저장소가 붙은 패키지명, 와일드카드, 그룹, 임의의 네이티브 옵션 전달은 초기 범위에 포함되지 않습니다. APT 스타일의 `list`는 기본적으로 설치된 패키지를 조회합니다. 업데이트 후보는 `list --upgradable`로 확인할 수 있습니다. 백엔드의 의존성 해결 정책과 종료 상태는 그대로 적용됩니다. 예를 들어 DNF의 `check-update`는 업데이트가 있을 때 종료 코드 `100`을 반환할 수 있습니다.

## 안전 수칙과 배포판별 동작 차이

- APT 스타일의 `update`는 메타데이터를 갱신하고, DNF 스타일의 `update`는 패키지를 업그레이드합니다.
- Pacman 백엔드의 `epa update`와 `-Sy`·`-Syy`는 root 소유의 별도 조회용 DB(`/var/cache/epa/pacman`)를 갱신하며 실제 설치용 DB를 변경하지 않습니다. 성공한 조회 갱신 결과는 검색·정보·업데이트 후보 및 자동완성에 사용합니다. 캐시가 없거나 갱신이 실패하면 네이티브 DB를 사용합니다. 설치·제거 등은 항상 시스템 DB를 사용합니다. `upgrade`, `full-upgrade` 및 `-Su`·`-Syu`는 네이티브 `pacman -Syu`로 시스템 전체를 갱신합니다. 이 과정에서 Pacman의 패키지 교체·제거 정책이 적용될 수 있습니다. 개별 패키지를 설치하기 전에는 Arch 시스템을 최신 상태로 유지하세요.
- APT 백엔드에서 Pacman 스타일의 `-Syu`는 `apt-get update`가 성공한 뒤 제거를 허용하지 않는 APT 업그레이드를 실행합니다. `dist-upgrade`가 아니며, APT의 `-Syy` 및 `-Syyu`는 HTTP(S)의 No-Cache 옵션으로 저장소를 다시 확인합니다. 변경되지 않은 로컬 인덱스까지 무조건 재다운로드하는 pacman의 강제 갱신과는 다릅니다. DNF에서는 `--refresh`를 사용합니다. Pacman 백엔드에서는 별도의 조회용 DB만 강제 갱신합니다.
- DNF 백엔드의 APT 스타일 `upgrade`는 `obsoletes`를 끄지만, 설치 전용 패키지 개수 제한 등 DNF의 트랜잭션 규칙은 적용됩니다. APT 스타일로 제거할 때 DNF의 자동 고아 패키지 제거는 끄지만, DNF 정책에 따라 대상에 의존하는 패키지가 함께 제거될 수 있습니다. EPA는 이 차이 때문에 DNF에서 Pacman 스타일 제거를 거부합니다.
- Alpine에서는 `epa update`가 APK 저장소 인덱스를 갱신하고, `upgrade`와 `full-upgrade`는 네이티브 `apk upgrade`를 실행합니다. `--available`은 자동으로 추가하지 않습니다. APT·DNF·Pacman 스타일의 설치·갱신·재설치는 APK의 명시적 확인 모드를 사용하며 자동 확인 옵션을 지정하면 `--no-interactive`를 전달합니다. APK 스타일(`epa style set apk`)에서는 네이티브 확인 정책을 따르며 `--interactive` 또는 `--no-interactive`로 변경할 수 있습니다.
- APK의 `del`은 world 제약과 불필요한 의존성을 함께 변경하며 다른 패키지가 요구하는 대상은 남겨둘 수 있습니다. 별도 `autoremove`와 Pacman 스타일 제거는 지원하지 않습니다. `epa --style apk cache clean`은 더 이상 필요한 것이 아닌 캐시 파일만 정리하므로 APT의 `clean`과 동일하게 취급하지 않습니다. APK 스타일의 `fix`는 명시된 패키지만 재설치하며, `info`에 대상이 없으면 설치된 패키지 이름을 조회합니다. 대상 지정 업그레이드, 가상 패키지 제약, 임의 플래그 전달은 지원하지 않습니다.
- 스타일과 백엔드가 다를 때 의미가 달라지는 대상 지정 업그레이드나 캐시 정리 명령은 거부합니다. 배포판마다 의존성 및 패키지 교체 정책이 다르므로 같은 명령이 동일한 변경을 보장하지는 않습니다.
- 패키지 이름 매핑, 서명 검증 우회, 임의 플래그 전달 및 셸 평가를 지원하지 않습니다.
- `--dry-run`은 실행할 명령과 인수를 보여줄 뿐, 의존성을 계산하거나 트랜잭션을 모의 실행하지는 않습니다. 특히 제거와 업그레이드 전에는 백엔드 확인 메시지를 읽으세요. `-y`와 `--noconfirm`은 네이티브 패키지 관리자에 자동 확인을 요청합니다.
- 자동화에서는 스타일을 명시하는 편이 안전합니다. 패키지 이름·버전 선택, 메타데이터, 잠금은 계속 백엔드가 관리합니다.

## 개발 및 검증

```sh
go test ./...
go vet ./...
go test -race ./...
go test -c -o /tmp/epa-pty.test ./internal/epa
python3 tests/pty_integration.py /tmp/epa-pty.test
go test ./internal/epa -run '^$' -fuzz '^FuzzParse$' -fuzztime 10s
go test ./internal/epa -run '^$' -fuzz '^FuzzParseProgress$' -fuzztime 10s
```

GitHub Actions에서 셸 자동완성, Linux 크로스 빌드, 배포판별 네이티브 트랜잭션을 검사합니다. 설치·제거를 실행하는 `tests/distro_smoke.sh`와 `tests/alpine_smoke.sh`는 폐기 가능한 컨테이너에서만 실행하세요. Alpine은 APK 2.x와 3.x를 각각 사용하는 Alpine 3.22 및 3.24 이미지를 검사합니다. 개발 버전이므로 운영 환경에 도입하기 전에는 대상 배포판과 패키지 관리자 버전에서 직접 검증하세요.

참고: [APT 매뉴얼](https://manpages.debian.org/bookworm/apt/apt-get.8.en.html), [DNF 명령 참조](https://dnf.readthedocs.io/en/latest/command_ref.html), [Pacman 매뉴얼](https://man.archlinux.org/man/pacman.8), [Alpine APK 공식 안내](https://docs.alpinelinux.org/user-handbook/0.1a/Working/apk.html), [Arch 시스템 유지보수 정책](https://wiki.archlinux.org/title/System_maintenance#Partial_upgrades_are_unsupported).

라이선스: GPL-3.0. [LICENSE](LICENSE)를 참고하세요.
