import http from 'k6/http';
import { check } from 'k6';

export const options = {
  vus: 1,
  duration: '5s'
};

export default function () {
  const response = http.get('http://127.0.0.1:8080/healthz');
  check(response, { 'health status is 200': (r) => r.status === 200 });
}
