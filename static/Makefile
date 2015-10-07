all: get-deps styles.css

get-deps: bower.json
	bower install

styles.css: styles.scss
	scss -t compressed styles.scss styles.css
