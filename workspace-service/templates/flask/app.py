# This should be the only entry point of the application
import os
from flask import Flask
from logging.config import dictConfig
from models import db
from flask_migrate import Migrate

dictConfig({
    'version': 1,
    'formatters': {'default': {
        'format': '[%(asctime)s] %(levelname)s in %(module)s: %(message)s',
    }},
    'handlers': {'wsgi': {
        'class': 'logging.StreamHandler',
        'stream': 'ext://sys.stdout',
        'formatter': 'default'
    }},
    'root': {
        'level': 'INFO',
        'handlers': ['wsgi']
    }
})

# Initialize Flask app
app = Flask(__name__, instance_path=os.path.join(os.getcwd(), 'instance'))
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///app.db'
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False

# Initialize the database
db.init_app(app)

# Initialize Flask-Migrate
migrate = Migrate(app, db)


# Add Code Here



# Run the application
if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=True)