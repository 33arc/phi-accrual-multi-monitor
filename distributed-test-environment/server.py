from flask import Flask, request, jsonify
import time
import sys
import random
import threading

app = Flask(__name__)

class ServerState:
    def __init__(self, delay):
        self.delay = delay

state = ServerState(
    delay= 0
)

@app.route('/heartbeat')
def heartbeat():
    time.sleep(state.delay)
    return 'OK'

@app.route('/control', methods=['GET', 'POST'])
def control():
    if request.method == 'GET':
        return jsonify({
            'delay': state.delay,
        })
    elif request.method == 'POST':
        data = request.json
        state.delay = float(data['delay'])
        return jsonify({'status': 'success', 'message': 'Server state updated'})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8080)
