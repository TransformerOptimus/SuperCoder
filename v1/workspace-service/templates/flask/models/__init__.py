import glob
import importlib
from os.path import basename, dirname, isfile, join
from flask_sqlalchemy import SQLAlchemy

db = SQLAlchemy()

# Dynamically import all models in the models directory
modules = glob.glob(join(dirname(__file__), "*.py"))
__all__ = [basename(f)[:-3] for f in modules if isfile(f) and not f.endswith("__init__.py")]

for module in __all__:
    importlib.import_module(f"models.{module}")
