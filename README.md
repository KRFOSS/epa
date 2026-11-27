# EPA — Everyone's Package Adapter

**Your commands. Any distribution.**

EPA는 ROKFOSS가 Go로 개발하는 실험적 리눅스 패키지 관리자 어댑터입니다. 배포판마다 다른 명령과 터미널 출력을 통합하되, 실제 작업은 운영체제에 설치된 패키지 관리자에 맡깁니다. 패키지 데이터베이스, 저장소 설정, 패키지 형식, 서명 검증, 의존성 해결기를 대체하지 않습니다.

패키지 이름은 자동 변환하지 않습니다. Debian의 `apache2`, RHEL의 `httpd`, Arch의 `apache`처럼 현재 배포판에서 제공하는 이름을 입력하세요.

## 빌드 및 설치

Linux와 Go 1.23 이상이 필요합니다. 외부 Go 라이브러리 없이 단일 실행 파일을 빌드합니다.

```sh
CGO_ENABLED=0 go build -trimpath -o epa ./cmd/epa
sudo install -m 0755 epa /usr/local/bin/epa
```

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

일반 사용자로 `epa`를 실행하세요. 시스템을 변경하는 작업에서는 네이티브 패키지 관리자만 `sudo`로 실행합니다. EPA 자체를 `sudo`로 실행해야 한다면 `--style`을 지정하세요. `sudo`로 호출한 EPA에서 스타일을 변경할 수는 없지만, root로 직접 로그인한 경우에는 root 계정의 스타일을 설정할 수 있습니다.

## 통합 터미널 출력

대화형 Linux 터미널에서 패키지를 변경하는 작업은 인라인 진행 화면을 사용합니다. APT, DNF/DNF5, Pacman은 같은 렌더러를 사용합니다. 인식한 백엔드 진행 기록을 스피너와 터미널 폭에 맞춘 진행 막대로 표시합니다. APT 다운로드에서 개별 항목 진행률과 전체 진행률을 모두 확인한 경우 두 수치를 구분합니다. 확인할 수 없는 수치는 추정하지 않으며, 명시적인 완료 기록이나 프로세스 종료 상태에 근거해 성공을 표시합니다.

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

출력 모드는 계정별로 저장하며, `--progress`로 한 번만 덮어쓸 수 있습니다. Linux 의사 터미널(PTY)을 통해 `sudo`, 패키지 관리자, 패키지 유지보수 스크립트가 입력을 받을 수 있게 합니다. 인식하지 못한 메시지, 줄바꿈 없는 프롬프트, 경고와 오류는 그대로 전달합니다. 복잡한 터미널 제어 시퀀스가 나오면 해당 작업의 나머지는 원본 출력으로 표시합니다. 렌더러가 확인 질문에 자동으로 응답하거나 비대화형 옵션을 추가하지는 않습니다.

터미널 크기 변경은 백엔드에도 전달합니다. 패키지 목록이 화면에 계속 쌓이지 않도록 현재 관측한 항목 하나를 갱신합니다. `NO_COLOR=1`로 EPA의 색상 출력을 끌 수 있습니다.

현재 진행 표시는 네이티브 CLI 출력 분석에 기반합니다. 배포판, 백엔드 버전, 로캘 또는 출력 형식에 따라 일부 진행 정보는 원본 텍스트로 표시됩니다. 예를 들어 DNF5의 여러 줄 출력은 원본 표시로 전환될 수 있습니다. 특수한 대화형 프로그램이나 터미널에서는 `native`를 사용하세요. `json`은 시작·종료 이벤트 스트림이며 패키지별 진행 이벤트를 제공하지 않습니다.

## 셸 자동완성

Bash에서는 시작 설정 파일에 추가하거나 현재 셸에서 한 번 실행하세요.

```bash
source <(epa completion bash)
```

Zsh에서는 `compinit`으로 자동완성을 초기화한 뒤 불러오세요.

```zsh
autoload -Uz compinit
compinit
source <(epa completion zsh)
```

자동완성은 활성 스타일의 명령과 옵션, 실제 백엔드의 패키지 이름을 제안합니다. 설치·검색·정보 조회에는 사용 가능한 패키지를, 제거·재설치에는 설치된 패키지를 제안합니다. 패키지 후보는 기존 로컬 메타데이터로만 조회하며 제한 시간은 2초입니다. 관리자 권한을 요청하지 않으며, 캐시가 없거나 읽을 수 없으면 패키지 후보를 표시하지 않습니다.

## 초기 지원 범위

| 명령 스타일 | 주요 명령 |
|---|---|
| APT | `update`, `upgrade`, `full-upgrade`, `install`, `remove`, `search`, `show`, `list`, `outdated`, `autoremove`, `reinstall`, `clean` |
| DNF | `makecache`, `update`/`upgrade`, `install`, `remove`/`erase`, `search`, `info`, `list`, `check-update`, `autoremove`, `reinstall`, `clean all` |
| Pacman | `-S`, `-Ss`, `-Si`, `-Sy`, `-Su`, `-Syu`, `-Syyu`, `-R`, `-Rs`, `-Rn`, `-Rns`, `-Q`, `-Qu`, `-Sc` |

백엔드는 APT, DNF(DNF5 포함), Pacman을 대상으로 합니다. `os-release`와 고정된 시스템 실행 파일 경로로 백엔드를 감지합니다. APK, Zypper, 구형 YUM은 아직 지원하지 않습니다.

EPA는 네이티브 명령 전체를 대체하지 않습니다. 알 수 없는 명령과 옵션은 오류로 처리합니다. 버전 지정자, 파일, URL, 저장소가 붙은 패키지명, 와일드카드, 그룹, 임의의 네이티브 옵션 전달은 초기 범위에 포함되지 않습니다. APT 스타일의 `list`는 기본적으로 설치된 패키지를 조회합니다. 업데이트 후보는 `list --upgradable`로 확인할 수 있습니다. 백엔드의 의존성 해결 정책과 종료 상태는 그대로 적용됩니다. 예를 들어 DNF의 `check-update`는 업데이트가 있을 때 종료 코드 `100`을 반환할 수 있습니다.

## 안전 수칙과 배포판별 동작 차이

- APT 스타일의 `update`는 메타데이터를 갱신하고, DNF 스타일의 `update`는 패키지를 업그레이드합니다.
- Pacman에서 메타데이터만 새로고침하면 부분 업그레이드가 발생할 수 있어 EPA는 이를 거부합니다. 또한 패키지 제거가 없음을 보장할 수 없어 Pacman 백엔드에서 APT 스타일의 `upgrade`를 거부합니다. 시스템 전체를 갱신하려면 `epa --style pacman -Syu` 또는 APT 스타일의 `epa full-upgrade`를 사용하세요. 개별 패키지를 설치하기 전에는 Arch 시스템을 최신 상태로 유지하세요.
- APT 백엔드에서 Pacman 스타일의 `-Syu`는 `apt-get update`가 성공한 뒤 제거를 허용하지 않는 APT 업그레이드를 실행합니다. `dist-upgrade`가 아니며, 다른 백엔드에서 강제 갱신 옵션에 안전한 대응 동작이 없으면 거부합니다.
- DNF 백엔드의 APT 스타일 `upgrade`는 `obsoletes`를 끄지만, 설치 전용 패키지 개수 제한 등 DNF의 트랜잭션 규칙은 적용됩니다. APT 스타일로 제거할 때 DNF의 자동 고아 패키지 제거는 끄지만, DNF 정책에 따라 대상에 의존하는 패키지가 함께 제거될 수 있습니다. EPA는 이 차이 때문에 DNF에서 Pacman 스타일 제거를 거부합니다.
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

GitHub Actions에서 셸 자동완성, Linux 크로스 빌드, 배포판별 네이티브 트랜잭션을 검사합니다. 설치·제거를 실행하는 `tests/distro_smoke.sh`는 폐기 가능한 컨테이너에서만 실행하세요. 개발 버전이므로 운영 환경에 도입하기 전에는 대상 배포판과 패키지 관리자 버전에서 직접 검증하세요.

참고: [APT 매뉴얼](https://manpages.debian.org/bookworm/apt/apt-get.8.en.html), [DNF 명령 참조](https://dnf.readthedocs.io/en/latest/command_ref.html), [Pacman 매뉴얼](https://man.archlinux.org/man/pacman.8), [Arch 시스템 유지보수 정책](https://wiki.archlinux.org/title/System_maintenance#Partial_upgrades_are_unsupported).

라이선스: GPL-3.0. [LICENSE](LICENSE)를 참고하세요.
