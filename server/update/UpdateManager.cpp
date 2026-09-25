#include "server/update/UpdateManager.h"

#include <array>
#include <cerrno>
#include <cstdlib>
#include <filesystem>
#include <sys/wait.h>
#include <unistd.h>
#include <vector>

namespace homeai {

namespace {

std::string trimCopy(
    std::string value
)
{
    while (
        !value.empty()
        &&
        (
            value.back() == '\n'
            ||
            value.back() == '\r'
            ||
            value.back() == ' '
            ||
            value.back() == '\t'
        )
    ) {
        value.pop_back();
    }

    std::size_t start = 0;

    while (
        start < value.size()
        &&
        (
            value[start] == ' '
            ||
            value[start] == '\t'
            ||
            value[start] == '\n'
            ||
            value[start] == '\r'
        )
    ) {
        ++start;
    }

    return value.substr(start);
}

std::string firstToken(
    const std::string& value
)
{
    const auto clean =
        trimCopy(value);

    const auto separator =
        clean.find_first_of(
            " \t\r\n"
        );

    if (
        separator ==
        std::string::npos
    ) {
        return clean;
    }

    return clean.substr(
        0,
        separator
    );
}

bool isBusyState(
    UpdateState state
)
{
    return
        state == UpdateState::Checking
        ||
        state == UpdateState::Updating
        ||
        state == UpdateState::Building
        ||
        state == UpdateState::Testing;
}

}

UpdateManager::~UpdateManager()
{
    stop();
}

bool UpdateManager::initialize(
    const std::string& repository_path,
    const std::string& remote,
    const std::string& branch,
    int check_interval_seconds
)
{
    if (
        repository_path.empty()
        ||
        remote.empty()
        ||
        branch.empty()
    ) {
        return false;
    }

    std::error_code error;

    const auto canonical =
        std::filesystem::canonical(
            repository_path,
            error
        );

    if (
        error
        ||
        !std::filesystem::exists(
            canonical / ".git"
        )
    ) {
        return false;
    }

    repository_path_ =
        canonical.string();

    remote_ = remote;
    branch_ = branch;

    check_interval_seconds_ =
        check_interval_seconds < 15
        ? 15
        : check_interval_seconds;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        status_ = {};
        status_.branch =
            branch_;

        status_.local_sha =
            gitHead();

        status_.state =
            UpdateState::Idle;

        status_.state_text =
            stateToString(
                status_.state
            );

        status_.message =
            "Ожидание проверки обновлений.";
    }

    return true;
}

void UpdateManager::start()
{
    if (running_)
        return;

    running_ = true;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        check_requested_ = true;
    }

    worker_ =
        std::thread(
            &UpdateManager::workerLoop,
            this
        );
}

void UpdateManager::stop()
{
    if (!running_)
        return;

    running_ = false;

    condition_.notify_all();

    if (worker_.joinable())
        worker_.join();
}

UpdateStatus UpdateManager::status() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    return status_;
}

void UpdateManager::requestCheck()
{
    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        if (
            isBusyState(
                status_.state
            )
        ) {
            return;
        }

        check_requested_ = true;
    }

    condition_.notify_all();
}

bool UpdateManager::requestUpdate(
    std::string& error
)
{
    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        if (
            isBusyState(
                status_.state
            )
        ) {
            error =
                "Обновление уже выполняется.";

            return false;
        }

        if (
            !status_.update_available
        ) {
            error =
                "Новая версия не обнаружена.";

            return false;
        }

        update_requested_ = true;
    }

    condition_.notify_all();

    return true;
}

bool UpdateManager::requestRestart(
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    if (
        status_.state !=
            UpdateState::ReadyToRestart
    ) {
        error =
            "Перезапуск доступен только после успешного обновления.";

        return false;
    }

    restart_requested_ = true;

    return true;
}

bool UpdateManager::consumeRestartRequest()
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    if (!restart_requested_)
        return false;

    restart_requested_ = false;

    return true;
}

std::string UpdateManager::restartBinaryPath() const
{
    return (
        std::filesystem::path(
            repository_path_
        )
        /
        "build/home-ai-core"
    ).string();
}

std::string UpdateManager::stateToString(
    UpdateState state
)
{
    switch (state) {
        case UpdateState::Idle:
            return "idle";
        case UpdateState::Checking:
            return "checking";
        case UpdateState::UpToDate:
            return "up_to_date";
        case UpdateState::UpdateAvailable:
            return "update_available";
        case UpdateState::Updating:
            return "updating";
        case UpdateState::Building:
            return "building";
        case UpdateState::Testing:
            return "testing";
        case UpdateState::ReadyToRestart:
            return "ready_to_restart";
        case UpdateState::Error:
            return "error";
    }

    return "unknown";
}

void UpdateManager::workerLoop()
{
    while (running_) {
        bool do_update = false;
        bool do_check = false;

        {
            std::unique_lock<std::mutex>
                lock(mutex_);

            condition_.wait_for(
                lock,
                std::chrono::seconds(
                    check_interval_seconds_
                ),
                [&]() {
                    return
                        !running_
                        ||
                        check_requested_
                        ||
                        update_requested_;
                }
            );

            if (!running_)
                break;

            if (update_requested_) {
                do_update = true;
                update_requested_ = false;
                check_requested_ = false;
            }
            else {
                do_check = true;
                check_requested_ = false;
            }
        }

        if (do_update)
            performUpdate();
        else if (do_check)
            performCheck();
    }
}

void UpdateManager::performCheck()
{
    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        if (
            status_.restart_required
        ) {
            return;
        }
    }

    setState(
        UpdateState::Checking,
        "Проверка GitHub..."
    );

    const auto local =
        gitHead();

    const auto remote =
        remoteHead();

    if (
        local.empty()
        ||
        remote.empty()
    ) {
        setState(
            UpdateState::Error,
            "Не удалось проверить обновления. Проверьте доступ Git/SSH."
        );

        return;
    }

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        status_.local_sha =
            local;

        status_.remote_sha =
            remote;

        status_.update_available =
            local != remote;

        status_.restart_required =
            false;

        status_.state =
            status_.update_available
            ? UpdateState::UpdateAvailable
            : UpdateState::UpToDate;

        status_.state_text =
            stateToString(
                status_.state
            );

        status_.message =
            status_.update_available
            ? "Доступна новая версия в GitHub."
            : "Сервер использует актуальную версию.";

        status_.busy = false;
    }
}

void UpdateManager::performUpdate()
{
    std::string dirty;

    if (!worktreeClean(dirty)) {
        setState(
            UpdateState::Error,
            "Обновление отменено: в репозитории есть локальные изменения.",
            dirty
        );

        return;
    }

    const auto old_head =
        gitHead();

    if (old_head.empty()) {
        setState(
            UpdateState::Error,
            "Не удалось определить текущую Git-версию."
        );

        return;
    }

    setState(
        UpdateState::Updating,
        "Получение изменений из GitHub..."
    );

    const auto pull =
        runCommand(
            "/usr/bin/git",
            {
                "pull",
                "--ff-only",
                remote_,
                branch_
            }
        );

    if (pull.exit_code != 0) {
        setState(
            UpdateState::Error,
            "Git pull завершился ошибкой.",
            pull.output
        );

        return;
    }

    const auto new_head =
        gitHead();

    if (new_head.empty()) {
        rollbackSource(
            old_head
        );

        setState(
            UpdateState::Error,
            "После обновления не удалось определить Git-версию."
        );

        return;
    }

    const auto next_build =
        std::filesystem::path(
            repository_path_
        )
        /
        "build-next";

    const auto active_build =
        std::filesystem::path(
            repository_path_
        )
        /
        "build";

    const auto previous_build =
        std::filesystem::path(
            repository_path_
        )
        /
        "build-prev";

    std::error_code fs_error;

    std::filesystem::remove_all(
        next_build,
        fs_error
    );

    setState(
        UpdateState::Building,
        "Сборка новой версии..."
    );

    const auto configure =
        runCommand(
            "/usr/bin/cmake",
            {
                "-S",
                repository_path_,
                "-B",
                next_build.string(),
                "-G",
                "Ninja"
            }
        );

    if (configure.exit_code != 0) {
        rollbackSource(
            old_head
        );

        std::filesystem::remove_all(
            next_build,
            fs_error
        );

        setState(
            UpdateState::Error,
            "CMake configuration failed. Репозиторий возвращён к предыдущей версии.",
            configure.output
        );

        return;
    }

    const auto build =
        runCommand(
            "/usr/bin/cmake",
            {
                "--build",
                next_build.string()
            }
        );

    if (build.exit_code != 0) {
        rollbackSource(
            old_head
        );

        std::filesystem::remove_all(
            next_build,
            fs_error
        );

        setState(
            UpdateState::Error,
            "Сборка завершилась ошибкой. Репозиторий возвращён к предыдущей версии.",
            build.output
        );

        return;
    }

    setState(
        UpdateState::Testing,
        "Запуск тестов новой версии..."
    );

    const auto tests =
        runCommand(
            "/usr/bin/ctest",
            {
                "--test-dir",
                next_build.string(),
                "--output-on-failure"
            }
        );

    if (tests.exit_code != 0) {
        rollbackSource(
            old_head
        );

        std::filesystem::remove_all(
            next_build,
            fs_error
        );

        setState(
            UpdateState::Error,
            "Тесты новой версии не прошли. Репозиторий возвращён к предыдущей версии.",
            tests.output
        );

        return;
    }

    std::filesystem::remove_all(
        previous_build,
        fs_error
    );

    fs_error.clear();

    if (
        std::filesystem::exists(
            active_build
        )
    ) {
        std::filesystem::rename(
            active_build,
            previous_build,
            fs_error
        );

        if (fs_error) {
            rollbackSource(
                old_head
            );

            setState(
                UpdateState::Error,
                "Не удалось сохранить предыдущую сборку.",
                fs_error.message()
            );

            return;
        }
    }

    fs_error.clear();

    std::filesystem::rename(
        next_build,
        active_build,
        fs_error
    );

    if (fs_error) {
        if (
            std::filesystem::exists(
                previous_build
            )
        ) {
            std::error_code restore_error;

            std::filesystem::rename(
                previous_build,
                active_build,
                restore_error
            );
        }

        rollbackSource(
            old_head
        );

        setState(
            UpdateState::Error,
            "Не удалось активировать новую сборку.",
            fs_error.message()
        );

        return;
    }

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        status_.state =
            UpdateState::ReadyToRestart;

        status_.state_text =
            stateToString(
                status_.state
            );

        status_.local_sha =
            new_head;

        status_.remote_sha =
            new_head;

        status_.update_available =
            false;

        status_.restart_required =
            true;

        status_.busy = false;

        status_.message =
            "Обновление установлено и протестировано. Требуется перезапуск.";

        status_.last_output =
            tests.output;
    }
}

UpdateManager::CommandResult
UpdateManager::runCommand(
    const std::string& executable,
    const std::vector<std::string>& arguments
) const
{
    CommandResult result;

    int pipe_fd[2]{};

    if (::pipe(pipe_fd) != 0) {
        result.output =
            "pipe failed";

        return result;
    }

    const pid_t child =
        ::fork();

    if (child < 0) {
        ::close(pipe_fd[0]);
        ::close(pipe_fd[1]);

        result.output =
            "fork failed";

        return result;
    }

    if (child == 0) {
        ::close(pipe_fd[0]);

        ::dup2(
            pipe_fd[1],
            STDOUT_FILENO
        );

        ::dup2(
            pipe_fd[1],
            STDERR_FILENO
        );

        ::close(pipe_fd[1]);

        ::chdir(
            repository_path_.c_str()
        );

        ::setenv(
            "GIT_TERMINAL_PROMPT",
            "0",
            1
        );

        std::vector<std::string>
            storage;

        storage.reserve(
            arguments.size() + 1
        );

        storage.push_back(
            executable
        );

        for (
            const auto& argument :
            arguments
        ) {
            storage.push_back(
                argument
            );
        }

        std::vector<char*>
            argv;

        argv.reserve(
            storage.size() + 1
        );

        for (auto& item : storage) {
            argv.push_back(
                item.data()
            );
        }

        argv.push_back(nullptr);

        ::execv(
            executable.c_str(),
            argv.data()
        );

        _exit(127);
    }

    ::close(pipe_fd[1]);

    std::string output;

    std::array<char, 2048>
        buffer{};

    while (output.size() < 65536) {
        const auto count =
            ::read(
                pipe_fd[0],
                buffer.data(),
                buffer.size()
            );

        if (count <= 0)
            break;

        output.append(
            buffer.data(),
            static_cast<std::size_t>(
                count
            )
        );
    }

    ::close(pipe_fd[0]);

    int status = 0;

    if (
        ::waitpid(
            child,
            &status,
            0
        ) < 0
    ) {
        result.output +=
            "\nwaitpid failed";

        return result;
    }

    result.output =
        trimCopy(output);

    if (WIFEXITED(status)) {
        result.exit_code =
            WEXITSTATUS(status);
    }

    return result;
}

bool UpdateManager::worktreeClean(
    std::string& details
) const
{
    const auto status =
        runCommand(
            "/usr/bin/git",
            {
                "status",
                "--porcelain",
                "--untracked-files=no"
            }
        );

    if (status.exit_code != 0) {
        details =
            status.output;

        return false;
    }

    details =
        status.output;

    return
        trimCopy(
            status.output
        ).empty();
}

std::string UpdateManager::gitHead() const
{
    const auto result =
        runCommand(
            "/usr/bin/git",
            {
                "rev-parse",
                "HEAD"
            }
        );

    if (result.exit_code != 0)
        return {};

    return firstToken(
        result.output
    );
}

std::string UpdateManager::remoteHead() const
{
    const auto result =
        runCommand(
            "/usr/bin/git",
            {
                "ls-remote",
                "--heads",
                remote_,
                "refs/heads/" +
                    branch_
            }
        );

    if (result.exit_code != 0)
        return {};

    return firstToken(
        result.output
    );
}

void UpdateManager::setState(
    UpdateState state,
    const std::string& message,
    const std::string& output
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    status_.state =
        state;

    status_.state_text =
        stateToString(state);

    status_.message =
        message;

    status_.last_output =
        output;

    status_.busy =
        isBusyState(state);

    if (
        state ==
        UpdateState::Error
    ) {
        status_.restart_required =
            false;
    }
}

void UpdateManager::rollbackSource(
    const std::string& old_head
)
{
    if (old_head.empty())
        return;

    runCommand(
        "/usr/bin/git",
        {
            "reset",
            "--hard",
            old_head
        }
    );
}

}
