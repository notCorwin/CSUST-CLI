from setuptools import find_packages, setup

from csust_cli import __version__


setup(
    name="csust-cli",
    version=__version__,
    description="长沙理工大学教务系统与 VPN 门户 CLI",
    python_requires=">=3.10",
    packages=find_packages(include=["csust_cli", "csust_cli.*"]),
    py_modules=["csust"],
    install_requires=["ddddocr>=1.5.6", "cryptography>=41"],
    entry_points={"console_scripts": ["csust=csust_cli.cli:main"]},
)
