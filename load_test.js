// load_test.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Кастомная метрика для отслеживания ошибок
const errorRate = new Rate('errors');

export const options = {
  // Сценарий: плавный рост нагрузки
  stages: [
    { duration: '10s', target: 10 },   
    { duration: '20s', target: 50 },  
    { duration: '40s', target: 200 },   
    { duration: '20s', target: 0 },    
  ],
  
  // Настройки
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export default function () {
  let shortCode = '';

  const baseUrl = __ENV.BASE_URL || 'http://localhost:8080/api';
  
  // POST /api/shorten 
  const payload = JSON.stringify({
    url: 'https://example.com/path/to/resource?param=value'
  });
  
  const postRes = http.post(`${baseUrl}/shorten`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });
  
  const postOk = check(postRes, {
    'POST /shorten status is 201': (r) => r.status === 201,
    'POST /shorten has shorten_url': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.shorten_url && body.shorten_url.length === 10;
      } catch {
        return false;
      }
    },
  });
  
  errorRate.add(!postOk);
  
  if (postOk) {
    shortCode = JSON.parse(postRes.body).shorten_url;
    
    // 2. GET api/original/:code 
    const getRes = http.get(`${baseUrl}/original/${shortCode}`);
    
    const getOk = check(getRes, {
      'GET /:code status is 200': (r) => r.status === 200,
      'GET /:code has original_url': (r) => {
        try {
          const body = JSON.parse(r.body);
          return body.original_url === 'https://example.com/path/to/resource?param=value';
        } catch {
          return false;
        }
      },
    });
    
    errorRate.add(!getOk);
  }
  
  sleep(0.1);
}