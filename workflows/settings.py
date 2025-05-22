import os

class Settings:
    ignored_versions: str
    version_file_path: str
    tests_to_trigger_file_path: str
    request_timeout_sec: int


    def __init__(self):
        self.ignored_versions = os.getenv("OCP_IGNORED_VERSIONS_REGEX", "x^").rstrip()
        self.version_file_path = os.getenv("VERSION_FILE_PATH")
        self.tests_to_trigger_file_path = os.getenv("TEST_TO_TRIGGER_FILE_PATH")
        self.request_timeout_sec = int(os.getenv("REQUEST_TIMEOUT_SECONDS", 30))

class DashboardSettings:
    dashboard_output_dir: str
    json_report_file: str
    dashboard_html_file: str

    def __init__(self):
        self.dashboard_output_dir = "workflows/test_matrix_dashboard/output"
        self.json_report_file = f"{self.dashboard_output_dir}/gpu_operator_matrix.json"
        self.dashboard_html_file = f"{self.dashboard_output_dir}/gpu_operator_matrix.html"

settings = Settings()
dashboard_settings = DashboardSettings()
