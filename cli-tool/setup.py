from distutils.core import setup

setup(
        name='servicegateway-cli',
        version='1.0.0',
        description='Command-line client for the Mittwald service gateway',
        url='https://github.com/mittwald/servicegateway',
        author='Martin Helmich',
        author_email='m.helmich@mittwald.de',
        license='GPL',
        classifiers=[],
        keywords='scanner detector cms extension plugin version',
        scripts=['svcgw'],
        install_requires=[
            'click==6.2',
            'python-consul==0.6.0',
            'requests==2.9.1'
        ]
)